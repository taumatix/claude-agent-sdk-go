package agent_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
)

// The stub-CLI end-to-end tests prove the SDK handles the frames this SDK
// believes the CLI emits. Only a real `claude` can say whether that belief is
// right — which is the failure mode this repo keeps finding: a suite that was
// green for six months because the fixtures asserted the shape the SDK
// invented on both sides.
//
// So this test spawns the installed binary through the ordinary subprocess
// transport, asks it to launch a subagent, and asserts on the lifecycle
// messages it actually sends back.
//
// It is opt-in because it needs credentials and spends real money on a model
// call. CI has neither, so CI does not run it:
//
//	CLAUDE_SDK_LIVE_E2E=1 go test ./domains/agent/ -run TestLive -v
//
// CLAUDE_SDK_LIVE_CLI_PATH points them at a particular `claude` binary instead of
// the one on PATH, so a newer CLI can be tried without installing it.
//
// Last run: 2026-09-25 against claude 2.1.267, passing.
func TestLive_TaskLifecycleFromRealCLI(t *testing.T) {
	cli := liveCLI(t)

	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, os.WriteFile(dir+"/"+name, nil, 0o600))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Deliberately not PermissionModeBypass. Bypass does not confine the
	// subprocess to WorkingDirectory — it hands a model-driven agent
	// unrestricted Bash on whatever machine runs the test. An allow-list is
	// enough to count files, and non-interactive mode denies the rest rather
	// than prompting.
	client := agent.NewClient(agent.Options{
		CLIPath:          cli,
		AllowedTools:     []string{"Task", "Glob", "Read", "Bash(ls:*)", "Bash(find:*)"},
		WorkingDirectory: dir,
	})
	require.NoError(t, client.Connect(ctx))
	defer func() { _ = client.Disconnect() }()

	prompt := "Launch one Explore subagent to count the files in " + dir +
		". Do not count them yourself."

	var (
		started   *messages.TaskStartedMessage
		terminal  bool
		lifecycle []string
	)
	for msg, err := range client.Query(ctx, prompt) {
		require.NoError(t, err)
		if msg.System != nil {
			lifecycle = append(lifecycle, msg.System.Subtype)
		}
		if msg.TaskStarted != nil && started == nil {
			started = msg.TaskStarted
		}
		if tu := msg.TaskUpdated; tu != nil && tu.Status.IsTerminal() {
			terminal = true
		}
		if tn := msg.TaskNotification; tn != nil && tn.Status.IsTerminal() {
			terminal = true
		}
	}

	require.NotNil(t, started,
		"the real CLI spawned no typed task_started; subtypes seen: %v", lifecycle)
	assert.NotEmpty(t, started.TaskID)
	assert.Equal(t, "local_agent", started.TaskType)
	assert.NotEmpty(t, started.SubagentType, "a Task-tool subagent reports its type")
	assert.True(t, terminal, "the task never reported a terminal status")
}

// session_state_changed and background_tasks_changed are typed from frames a
// single probe produced; this asserts them against the installed binary through
// the real client, with the opt-in the SessionStateChangedMessage doc tells
// callers to use.
//
// Since 0.6.0 a Query waits for "idle" when the CLI reports state, so the opted-
// in caller now sees it, last, inside the Query. Before, on 2.1.283 it came
// after the result, where Query had already returned.
//
// Last run: 2026-10-03 against claude 2.1.283 and 2.1.288, passing.
func TestLive_SessionStateAndBackgroundTasksFromRealCLI(t *testing.T) {
	cli := liveCLI(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// The allow-list admits `sleep` and the tools that read a background
	// shell's output, nothing else.
	client := agent.NewClient(agent.Options{
		CLIPath:          cli,
		AllowedTools:     []string{"Bash(sleep:*)", "BashOutput", "TaskOutput"},
		WorkingDirectory: t.TempDir(),
		Env:              map[string]string{"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS": "1"},
	})
	require.NoError(t, client.Connect(ctx))
	defer func() { _ = client.Disconnect() }()

	prompt := "Run the Bash command `sleep 3` with run_in_background set to true, " +
		"then wait for it to finish and reply with exactly: DONE"

	var (
		states      []messages.SessionState
		sawLiveBash bool
		emptyAfter  bool
		subtypes    []string
	)
	for msg, err := range client.Query(ctx, prompt) {
		require.NoError(t, err)
		if msg.System != nil {
			subtypes = append(subtypes, msg.System.Subtype)
		}
		if s := msg.SessionStateChanged; s != nil {
			states = append(states, s.State)
		}
		if b := msg.BackgroundTasksChanged; b != nil {
			for _, task := range b.Tasks {
				if task.TaskType == "local_bash" && task.TaskID != "" {
					sawLiveBash = true
				}
			}
			if sawLiveBash && len(b.Tasks) == 0 {
				emptyAfter = true
			}
		}
	}

	require.NotEmpty(t, states, "no typed session_state_changed arrived; subtypes seen: %v", subtypes)
	assert.Equal(t, messages.SessionStateRunning, states[0], "a turn starts by reporting running")
	assert.Equal(t, messages.SessionStateIdle, states[len(states)-1],
		"the Query ended before the CLI said idle; states seen: %v", states)
	for _, s := range states {
		assert.Contains(t, []messages.SessionState{
			messages.SessionStateIdle, messages.SessionStateRunning, messages.SessionStateRequiresAction,
		}, s, "the CLI reported a state this SDK does not name")
	}
	assert.True(t, sawLiveBash, "no typed background_tasks_changed listed the shell; subtypes seen: %v", subtypes)
	assert.True(t, emptyAfter, "the set never returned to empty after the shell finished")
}

// The claim that SystemMessage.Data is dead is the premise of the whole slice,
// and it is a claim about the CLI rather than about this SDK. This asserts it
// against the installed binary so a future CLI that starts nesting payloads is
// caught here rather than by a user.
func TestLive_NoSystemMessageCarriesADataKey(t *testing.T) {
	cli := liveCLI(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := agent.NewClient(agent.Options{CLIPath: cli})
	require.NoError(t, client.Connect(ctx))
	defer func() { _ = client.Disconnect() }()

	seen := 0
	for msg, err := range client.Query(ctx, "reply with exactly: OK") {
		require.NoError(t, err)
		if msg.System == nil {
			continue
		}
		seen++
		// No opt-in here, so the doc's claim that session state is opt-in is
		// checked against the binary on every live run.
		assert.Nil(t, msg.SessionStateChanged,
			"session_state_changed arrived without CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS; the opt-in note is wrong")
		// Asserted on the length, not the value: a failure here must name the
		// subtype without printing a live session's payload into the log.
		//lint:ignore SA1019 asserting the deprecated field is empty is the test.
		assert.Zero(t, len(msg.System.Data),
			"subtype %q carried a `data` key; SystemMessage.Data is no longer dead and the "+
				"deprecation note needs revisiting", msg.System.Subtype)
		assert.NotEmpty(t, msg.System.Raw, "subtype %q reached the caller with no Raw", msg.System.Subtype)
	}

	require.Positive(t, seen, "no system messages arrived at all, so nothing was checked")
}

// liveCLI skips unless live tests were asked for, and returns the `claude` to
// run: CLAUDE_SDK_LIVE_CLI_PATH if set, otherwise the one on PATH.
func liveCLI(t *testing.T) string {
	t.Helper()
	if os.Getenv("CLAUDE_SDK_LIVE_E2E") != "1" {
		t.Skip("set CLAUDE_SDK_LIVE_E2E=1 to run against a real `claude` (needs credentials, costs money)")
	}
	if p := os.Getenv("CLAUDE_SDK_LIVE_CLI_PATH"); p != "" {
		return p
	}
	p, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("no `claude` on PATH")
	}
	return p
}

// On a CLI that reports no session state (2.1.283 without the caller's
// opt-in), the task ledger is what keeps a Query open while a background agent
// runs. The guarantee: an agent the Query saw start is seen to finish before
// the Query ends. Before 0.7.0 the Query ended at the first result and the
// agent's end, and its follow-up turn, went to nobody.
//
// Last run: 2026-10-04 against claude 2.1.283, passing.
func TestLive_AQueryOutlastsTheBackgroundAgentItStarted(t *testing.T) {
	cli := liveCLI(t)

	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		require.NoError(t, os.WriteFile(dir+"/"+name, nil, 0o600))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	started := map[string]bool{}
	finished := map[string]bool{}
	results := 0
	for msg, err := range agent.Query(ctx,
		"Launch one Explore subagent with run_in_background set to true to count the files in "+dir+
			". Do not wait for it: reply with exactly STARTED, and nothing else, as soon as it is launched.",
		agent.Options{
			CLIPath:          cli,
			AllowedTools:     []string{"Task", "Glob", "Bash(ls:*)"},
			WorkingDirectory: dir,
		}) {
		require.NoError(t, err)
		if ts := msg.TaskStarted; ts != nil && ts.TaskType == "local_agent" {
			started[ts.TaskID] = true
		}
		if tn := msg.TaskNotification; tn != nil && tn.Status.IsTerminal() {
			finished[tn.TaskID] = true
		}
		if tu := msg.TaskUpdated; tu != nil && tu.Status.IsTerminal() {
			finished[tu.TaskID] = true
		}
		if msg.Result != nil {
			results++
		}
		if b := msg.BackgroundTasksChanged; b != nil {
			var listed []string
			for _, task := range b.Tasks {
				listed = append(listed, task.TaskID+":"+task.TaskType)
			}
			t.Logf("background_tasks_changed %v", listed)
		}
	}

	require.NotEmpty(t, started, "the model launched no agent, so this run proves nothing")
	for id := range started {
		assert.True(t, finished[id], "the Query ended with agent %s still running", id)
	}
	t.Logf("agents %d, results %d", len(started), results)
}

// A deny rule refuses a Bash call, and the refusal reaches the caller as a
// PermissionDeniedMessage naming the same tool call the result lists.
func TestLive_ADenyRuleIsReportedAsPermissionDenied(t *testing.T) {
	cli := liveCLI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var denied []*messages.PermissionDeniedMessage
	var result *messages.ResultMessage
	maxTurns := 3
	for msg, err := range agent.Query(ctx,
		"Run the shell command `echo hi` with the Bash tool and tell me its output. If it is refused, just say REFUSED.",
		agent.Options{
			CLIPath:          cli,
			DisallowedTools:  []string{"Bash(echo:*)"},
			MaxTurns:         &maxTurns,
			WorkingDirectory: t.TempDir(),
		}) {
		require.NoError(t, err)
		if msg.PermissionDenied != nil {
			denied = append(denied, msg.PermissionDenied)
		}
		if msg.Result != nil {
			result = msg.Result
		}
	}

	require.NotEmpty(t, denied, "the model did not try Bash, or the denial was not typed")
	assert.Equal(t, "Bash", denied[0].ToolName)
	assert.NotEmpty(t, denied[0].ToolUseID)
	assert.NotEmpty(t, denied[0].Message)
	require.NotNil(t, result)
	t.Logf("denied %+v", *denied[0])
}
