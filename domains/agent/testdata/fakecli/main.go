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
// The last two matter most. `task_updated` carries the terminal status for a
// task whose notification can be suppressed, and `background_tasks_changed` is
// here to prove an unmodelled subtype still reaches the caller.
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
	`{"type":"system","subtype":"background_tasks_changed","tasks":[],"uuid":"u-bg","session_id":"e2e"}`,
}

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
			for _, line := range lifecycle {
				emit(line)
			}
			emit(compact(turn))
			emit(result)
		}
	}
}

// compact strips the newlines that keep the turn literal readable, so it goes
// out as one JSON line.
func compact(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		panic(err)
	}
	return buf.String()
}
