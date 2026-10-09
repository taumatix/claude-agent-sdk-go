// Command fakecli is a stand-in for the `claude` binary, used by the
// end-to-end test in domains/agent. It speaks the same stdio control protocol
// over real pipes: answer the initialize control_request, then reply to a user
// message with a turn whose content blocks are the ones the real CLI emits for
// a server-side tool call.
//
// The block types below are copied from the content-block switch in the
// `claude` binary (2.1.220) — `server_tool_use`, `web_search_tool_result`,
// `mcp_tool_use`. They are not what this SDK invented; see
// domains/protocol/types.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// turn is the reply to any user message: a server-side search whose result
// block is the one the SDK used to drop, plus a block no SDK models.
const turn = `{"type":"assistant","session_id":"e2e","model":"claude-sonnet-5","message":{"role":"assistant","content":[
{"type":"server_tool_use","id":"srvtoolu_01","name":"web_search","input":{"query":"go 1.26 release date"}},
{"type":"web_search_tool_result","tool_use_id":"srvtoolu_01","content":[{"type":"web_search_result","title":"Go 1.26","url":"https://go.dev/doc/devel/release"}]},
{"type":"mcp_tool_use","id":"mcptoolu_01","name":"lookup","server_name":"docs","input":{"q":"iter.Seq2"}},
{"type":"text","text":"Go 1.26 is out."}
]}}`

// lifecycle is the system-message sequence a turn that spawns a subagent emits,
// in the order a real run produced it. These lines are the frames captured from
// `claude` 2.1.267 on 2026-09-25 (ids shortened, hook output replaced with a
// short string): a hook pair around session start, then the four task
// lifecycle events, then one subtype this SDK does not model.
//
// `task_updated` carries the terminal status for a task whose notification can
// be suppressed. The background set filling and emptying is the 2.1.283 shape
// (2026-09-27), and `thinking_tokens` is here to prove an unmodelled subtype
// still reaches the caller.
var lifecycle = []string{
	`{"type":"system","subtype":"hook_started","hook_id":"hk-1","hook_name":"SessionStart:startup",` +
		`"hook_event":"SessionStart","uuid":"u-hs","session_id":"e2e"}`,
	`{"type":"system","subtype":"hook_progress","hook_id":"hk-1","hook_name":"SessionStart:startup",` +
		`"hook_event":"SessionStart","stdout":"on stdout","stderr":"on stderr","output":"the output",` +
		`"uuid":"u-hp","session_id":"e2e"}`,
	`{"type":"system","subtype":"hook_response","hook_id":"hk-1","hook_name":"SessionStart:startup",` +
		`"hook_event":"SessionStart","output":"the output","stdout":"on stdout","stderr":"on stderr",` +
		`"exit_code":0,"outcome":"success","uuid":"u-hr","session_id":"e2e"}`,
	`{"type":"system","subtype":"task_started","task_id":"tk-1","tool_use_id":"toolu_01",` +
		`"description":"Count files","subagent_type":"Explore","is_backgrounded":false,"spawn_depth":1,` +
		`"task_type":"local_agent","prompt":"Count the files.","uuid":"u-ts","session_id":"e2e"}`,
	`{"type":"system","subtype":"task_progress","task_id":"tk-1","tool_use_id":"toolu_01",` +
		`"description":"Running","subagent_type":"Explore",` +
		`"usage":{"total_tokens":12815,"tool_uses":1,"duration_ms":2200},` +
		`"last_tool_name":"Bash","uuid":"u-tp","session_id":"e2e"}`,
	`{"type":"system","subtype":"task_updated","task_id":"tk-1",` +
		`"patch":{"status":"completed","end_time":1790295641558},"uuid":"u-tu","session_id":"e2e"}`,
	`{"type":"system","subtype":"task_notification","task_id":"tk-1","tool_use_id":"toolu_01",` +
		`"status":"completed","output_file":"/tmp/tasks/tk-1.output","summary":"3 files.",` +
		`"usage":{"total_tokens":12815,"tool_uses":1,"duration_ms":2200},"uuid":"u-tn","session_id":"e2e"}`,
	// A second task, started and killed, so the tests cannot pass by treating
	// one task id as "the" task — and so the terminal-only-via-task_updated
	// case is exercised: tk-2 never gets a task_notification.
	`{"type":"system","subtype":"task_started","task_id":"tk-2","tool_use_id":"toolu_02",` +
		`"description":"Watch the log","task_type":"local_bash","is_backgrounded":true,` +
		`"uuid":"u-ts2","session_id":"e2e"}`,
	`{"type":"system","subtype":"task_updated","task_id":"tk-2",` +
		`"patch":{"status":"killed","end_time":1790295642000,"error":"stopped by TaskStop"},` +
		`"uuid":"u-tu2","session_id":"e2e"}`,
	`{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"bmrd57x9h",` +
		`"task_type":"local_bash","description":"Wait three seconds then print done"}],` +
		`"uuid":"u-bg-1","session_id":"e2e"}`,
	`{"type":"system","subtype":"background_tasks_changed","tasks":[],"uuid":"u-bg","session_id":"e2e"}`,
	`{"type":"system","subtype":"thinking_tokens","estimated_tokens":50,"estimated_tokens_delta":50,` +
		`"session_id":"e2e","uuid":"u-tt"}`,
	`{"type":"system","subtype":"compact_boundary","content":"Conversation compacted",` +
		`"compact_metadata":{"trigger":"manual","pre_tokens":33919,"post_tokens":3337,"duration_ms":12564},` +
		`"session_id":"e2e","uuid":"u-cb"}`,
}

// Like the real CLI, session_state_changed is sent only when the caller opted
// in through the environment, so a test can prove Options.Env reaches the
// subprocess and the frame reaches the caller typed.
const (
	sessionRunning = `{"type":"system","subtype":"session_state_changed","state":"running","uuid":"u-ssr","session_id":"e2e"}`
	sessionIdle    = `{"type":"system","subtype":"session_state_changed","state":"idle","uuid":"u-ssi","session_id":"e2e"}`
)

const result = `{"type":"result","subtype":"success","session_id":"e2e","duration_ms":12,` +
	`"duration_api_ms":9,"is_error":false,"num_turns":1,"result":"Go 1.26 is out.","terminal_reason":"completed"}`

func main() {
	// A `-v` probe from the transport's version check, not a protocol session.
	for _, arg := range os.Args[1:] {
		if arg == "-v" || arg == "--version" {
			fmt.Println("2.1.220 (Claude Code)")
			return
		}
	}

	if f := os.Getenv("FAKECLI_ARGS_FILE"); f != "" {
		_ = os.WriteFile(f, []byte(strings.Join(os.Args[1:], "\n")), 0o600)
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	emit := func(line string) {
		fmt.Fprintln(out, line)
		out.Flush()
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var frame struct {
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}

		switch frame.Type {
		case "control_request":
			emit(fmt.Sprintf(
				`{"type":"control_response","response":{"subtype":"success","request_id":%q,"response":{}}}`,
				frame.RequestID))
		case "user":
			// CLAUDE_CODE_SDK_READS_SESSION_STATE is how the SDK asks for
			// session state for itself. 2.1.288 answers it with frames marked
			// sdk_host_only, "running" before the turn and "idle" after the
			// result (probed 2026-10-03); 2.1.283 ignores it. The stub plays
			// 2.1.288 unless FAKECLI_SCENARIO=old-cli.
			scenario := os.Getenv("FAKECLI_SCENARIO")
			hostState := os.Getenv("CLAUDE_CODE_SDK_READS_SESSION_STATE") != "" && !strings.HasPrefix(scenario, "old-cli")
			sessionState := os.Getenv("CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS") != ""
			// With both set, 2.1.288 sends one set of frames, visible to the
			// caller, and "idle" after the result (probed 2026-10-03).
			both := hostState && sessionState
			switch {
			case both, sessionState:
				emit(sessionRunning)
			case hostState:
				emit(hostRunning)
			}
			for _, line := range lifecycle {
				// FAKECLI_NO_LEVEL drops the level frame, which otherwise
				// empties the SDK's ledger every turn and hides whether a
				// reconnect did.
				if os.Getenv("FAKECLI_NO_LEVEL") != "" && strings.Contains(line, `"background_tasks_changed"`) {
					continue
				}
				emit(line)
			}
			emit(compact(turn))
			if sessionState && !both {
				// The 2.1.283 shape with only the caller's opt-in.
				emit(sessionIdle)
			}
			switch scenario {
			case "old-cli-agent":
				// 2.1.283 without session state: an agent launched in the
				// background is still running at the result. It finishes
				// afterwards, the CLI asks a hook to run for it and answers the
				// follow-up turn it woke for. The task frames are the SDK's only
				// sign that any of that is coming.
				// Level frames where 2.1.283 and 2.1.288 send them: the agent
				// listed as it starts, and an empty list just before its end.
				emit(levelAgent)
				emit(bgAgentStarted)
				emit(result)
				emit(`{"type":"control_request","request_id":"hook-after-result",` +
					`"request":{"subtype":"hook_callback","callback_id":"hook_0","input":{"hook_event_name":"SubagentStop"}}}`)
				if !awaitResponse(scanner, "hook-after-result") {
					return
				}
				emit(levelEmpty)
				emit(bgAgentDone)
				emit(followupTurn)
				emit(followupResult)
				continue
			case "old-cli-agent-level-stale":
				// The agent stays listed, alongside a shell, and nothing ends it.
				emit(levelAgent)
				emit(bgAgentStarted)
				emit(result)
				emit(`{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"tk-bg",` +
					`"task_type":"local_agent","description":"Background review"},{"task_id":"sh-bg",` +
					`"task_type":"local_bash","description":"tail -f log"}],"uuid":"u-lv-s","session_id":"e2e"}`)
				continue
			case "old-cli-agent-bookend-lost":
				// The agent's end bookend never arrives, but the level does:
				// the CLI's own answer to a lost bookend. The follow-up turn
				// follows as usual.
				emit(levelAgent)
				emit(bgAgentStarted)
				emit(result)
				emit(levelEmpty)
				emit(followupTurn)
				emit(followupResult)
				continue
			case "old-cli-agent-lost-once":
				// The first process to see a prompt loses its agent; any later
				// one answers plainly, so a test can tell a reconnect started
				// a clean session. FAKECLI_STATE_FILE marks the first.
				if f := os.Getenv("FAKECLI_STATE_FILE"); f != "" {
					if _, err := os.Stat(f); err != nil {
						_ = os.WriteFile(f, nil, 0o600)
						emit(bgAgentStarted)
					}
				}
				emit(result)
				continue
			case "old-cli-agent-lost":
				// An agent starts in the background and its end never arrives:
				// the CLI crashed, the frame was lost, or a later CLI renamed
				// the subtype. Nothing more is sent for this prompt.
				emit(bgAgentStarted)
				emit(result)
				continue
			case "idle-every-turn":
				// A CLI that reports "idle" at the end of every turn, background
				// work or not. Upstream's comment says such CLIs exist. The
				// agent then finishes, its hook runs, and the follow-up turn it
				// woke for ends in a result and another "idle".
				emit(bgAgentStarted)
				emit(result)
				emit(hostIdle)
				emit(`{"type":"control_request","request_id":"hook-after-idle",` +
					`"request":{"subtype":"hook_callback","callback_id":"hook_0","input":{"hook_event_name":"SubagentStop"}}}`)
				if !awaitResponse(scanner, "hook-after-idle") {
					return
				}
				emit(bgAgentDone)
				emit(followupTurn)
				emit(followupResult)
				emit(hostIdle)
				continue
			case "old-cli-api-retry":
				emit(`{"type":"system","subtype":"api_retry","attempt":1,"max_retries":2,"retry_delay_ms":595,` +
					`"error_status":529,"error":"overloaded","session_id":"e2e","uuid":"u-ar"}`)
				emit(result)
				continue
			case "old-cli-permission-denied":
				// A deny rule refused a Bash call: the frame as 2.1.288 sends it.
				emit(`{"type":"system","subtype":"permission_denied","tool_name":"Bash",` +
					`"tool_use_id":"toolu_pd","decision_reason_type":"subcommandResults",` +
					`"message":"Permission to use Bash with command echo hi has been denied.",` +
					`"uuid":"u-pd","session_id":"e2e"}`)
				emit(result)
				continue
			case "old-cli-shell":
				// A background shell is still running at the result and never
				// reports an end. Shells are not waited for: one can run for
				// ever, and waiting would hang the Query.
				emit(bgShellStarted)
				emit(result)
				continue
			}
			emit(result)

			switch scenario {
			case "followup":
				// A background agent finished after the result: the CLI asks a
				// hook to run, and answers the follow-up turn it woke for, all
				// before "idle". An SDK that closed stdin at the first result
				// never answers the hook.
				emit(`{"type":"control_request","request_id":"hook-after-result",` +
					`"request":{"subtype":"hook_callback","callback_id":"hook_0","input":{"hook_event_name":"SubagentStop"}}}`)
				if !awaitResponse(scanner, "hook-after-result") {
					return
				}
				emit(followupTurn)
				emit(followupResult)
			case "noidle":
				// State was reported, but "idle" never comes: the SDK must
				// give up at its ceiling rather than wait for ever.
				continue
			}
			switch {
			case both:
				emit(sessionIdle)
			case hostState:
				emit(hostIdle)
			}
		}
	}
}

// awaitResponse reads stdin until the SDK answers requestID, answering any
// control request of its own in the meantime. It reports false on EOF, which
// is what an SDK that closed stdin early looks like from here.
func awaitResponse(scanner *bufio.Scanner, requestID string) bool {
	for scanner.Scan() {
		var frame struct {
			Type     string `json:"type"`
			Response struct {
				RequestID string `json:"request_id"`
			} `json:"response"`
		}
		if json.Unmarshal(scanner.Bytes(), &frame) == nil &&
			frame.Type == "control_response" && frame.Response.RequestID == requestID {
			return true
		}
	}
	return false
}

const (
	// Shapes from the 2.1.267 lifecycle frames above, with ids of their own.
	bgAgentStarted = `{"type":"system","subtype":"task_started","task_id":"tk-bg","tool_use_id":"toolu_bg",` +
		`"description":"Background review","subagent_type":"Explore","is_backgrounded":true,"spawn_depth":1,` +
		`"task_type":"local_agent","uuid":"u-bg-s","session_id":"e2e"}`
	bgAgentDone = `{"type":"system","subtype":"task_notification","task_id":"tk-bg","tool_use_id":"toolu_bg",` +
		`"status":"completed","output_file":"/tmp/tk-bg.output","summary":"Reviewed.","uuid":"u-bg-n","session_id":"e2e"}`
	levelAgent = `{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"tk-bg",` +
		`"task_type":"local_agent","description":"Background review"}],"uuid":"u-lv-a","session_id":"e2e"}`
	levelEmpty     = `{"type":"system","subtype":"background_tasks_changed","tasks":[],"uuid":"u-lv-e","session_id":"e2e"}`
	bgShellStarted = `{"type":"system","subtype":"task_started","task_id":"sh-bg","description":"tail -f log",` +
		`"is_backgrounded":true,"task_type":"local_bash","uuid":"u-sh-s","session_id":"e2e"}`

	hostRunning = `{"type":"system","subtype":"session_state_changed","state":"running","sdk_host_only":true,"uuid":"u-hr","session_id":"e2e"}`
	hostIdle    = `{"type":"system","subtype":"session_state_changed","state":"idle","sdk_host_only":true,"uuid":"u-hi","session_id":"e2e"}`

	followupTurn = `{"type":"assistant","session_id":"e2e","model":"claude-sonnet-5","message":{"role":"assistant",` +
		`"content":[{"type":"text","text":"The background agent finished."}]}}`
	followupResult = `{"type":"result","subtype":"success","session_id":"e2e","duration_ms":5,` +
		`"duration_api_ms":4,"is_error":false,"num_turns":2,"result":"The background agent finished.","terminal_reason":"completed"}`
)

// compact strips the newlines that keep the turn literal readable, so it goes
// out as one JSON line.
func compact(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		panic(err)
	}
	return buf.String()
}
