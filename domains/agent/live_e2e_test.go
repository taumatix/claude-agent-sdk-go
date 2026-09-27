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
// Last run: 2026-09-25 against claude 2.1.267, passing.
func TestLive_TaskLifecycleFromRealCLI(t *testing.T) {
	if os.Getenv("CLAUDE_SDK_LIVE_E2E") != "1" {
		t.Skip("set CLAUDE_SDK_LIVE_E2E=1 to run against the installed `claude` (needs credentials, costs money)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no `claude` on PATH")
	}

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
// "idle" is not asserted. On 2.1.283 it arrives after the follow-up turn the
// finished background task wakes, which is after the first result, where Query
// returns. Reaching it is a roadmap entry, not something this test can see.
//
// Last run: 2026-09-27 against claude 2.1.283, passing.
func TestLive_SessionStateAndBackgroundTasksFromRealCLI(t *testing.T) {
	if os.Getenv("CLAUDE_SDK_LIVE_E2E") != "1" {
		t.Skip("set CLAUDE_SDK_LIVE_E2E=1 to run against the installed `claude` (needs credentials, costs money)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no `claude` on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// The allow-list admits `sleep` and the tools that read a background
	// shell's output, nothing else.
	client := agent.NewClient(agent.Options{
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
	if os.Getenv("CLAUDE_SDK_LIVE_E2E") != "1" {
		t.Skip("set CLAUDE_SDK_LIVE_E2E=1 to run against the installed `claude` (needs credentials, costs money)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no `claude` on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := agent.NewClient(agent.Options{})
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
