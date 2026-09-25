package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
)

// These drive the whole stack over a real subprocess and real pipes: the stub
// CLI emits the lifecycle frames a real `claude` 2.1.267 run produced, and the
// assertions are made on what a caller of client.Query actually receives.
//
// Unit tests in domains/messages cover the same decoding, but they hand the
// parser a byte slice. Only this path proves the frames survive the transport,
// the read loop and the message iterator — the layer where the Disconnect
// deadlock and the dropped server-tool result both lived.

// TestE2E_TaskLifecycleReachesTheCaller is the case the roadmap entry was
// written for: following a subagent from spawn to terminal state without
// hand-decoding anything.
func TestE2E_TaskLifecycleReachesTheCaller(t *testing.T) {
	got := queryFakeCLI(t, "count the files")

	var (
		started      *messages.TaskStartedMessage
		progress     *messages.TaskProgressMessage
		updated      *messages.TaskUpdatedMessage
		notification *messages.TaskNotificationMessage
	)
	// Selected by task id, not "the last one seen": the stub emits two tasks,
	// and tk-2 is deliberately a different shape (no subagent, no spawn depth).
	for i := range got {
		switch {
		case got[i].TaskStarted != nil && got[i].TaskStarted.TaskID == "tk-1":
			started = got[i].TaskStarted
		case got[i].TaskProgress != nil:
			progress = got[i].TaskProgress
		case got[i].TaskUpdated != nil && got[i].TaskUpdated.TaskID == "tk-1":
			updated = got[i].TaskUpdated
		case got[i].TaskNotification != nil:
			notification = got[i].TaskNotification
		}
	}

	require.NotNil(t, started, "task_started never reached the caller as a typed message")
	assert.Equal(t, "tk-1", started.TaskID)
	assert.Equal(t, "Explore", started.SubagentType)
	assert.Equal(t, "local_agent", started.TaskType)
	assert.Equal(t, "toolu_01", started.ToolUseID)
	require.NotNil(t, started.SpawnDepth)
	assert.Equal(t, 1, *started.SpawnDepth)
	require.NotNil(t, started.IsBackgrounded)
	assert.False(t, *started.IsBackgrounded)

	require.NotNil(t, progress, "task_progress never reached the caller")
	assert.Equal(t, "Bash", progress.LastToolName)
	assert.Equal(t, 12815, progress.Usage.TotalTokens)

	require.NotNil(t, updated, "task_updated never reached the caller")
	assert.Equal(t, messages.TaskStatusCompleted, updated.Status)
	assert.True(t, updated.Status.IsTerminal())

	require.NotNil(t, notification, "task_notification never reached the caller")
	assert.Equal(t, messages.TaskStatusCompleted, notification.Status)
	assert.Equal(t, "3 files.", notification.Summary)
	require.NotNil(t, notification.Usage)
	assert.Equal(t, 1, notification.Usage.ToolUses)
}

// A caller tracking active tasks needs only the terminal check, not knowledge
// of which of the two messages happens to carry it. This walks the stream the
// way such a caller would.
//
// The emptiness assertion alone was vacuous: with task_started decoding removed
// the map was never populated, stayed empty, and the test passed. So the tasks
// seen are counted and asserted too — the stub emits two, and tk-2 reaches its
// terminal state *only* through task_updated, which is the case the entry
// exists for.
func TestE2E_TaskCanBeTrackedToTerminalByStatusAlone(t *testing.T) {
	got := queryFakeCLI(t, "count the files")

	active := map[string]bool{}
	var everActive []string
	clearedBy := map[string]string{}

	for i := range got {
		if ts := got[i].TaskStarted; ts != nil {
			active[ts.TaskID] = true
			everActive = append(everActive, ts.TaskID)
		}
		if tu := got[i].TaskUpdated; tu != nil && tu.Status.IsTerminal() {
			if active[tu.TaskID] {
				clearedBy[tu.TaskID] = "task_updated"
			}
			delete(active, tu.TaskID)
		}
		if tn := got[i].TaskNotification; tn != nil && tn.Status.IsTerminal() {
			if active[tn.TaskID] {
				clearedBy[tn.TaskID] = "task_notification"
			}
			delete(active, tn.TaskID)
		}
	}

	require.Equal(t, []string{"tk-1", "tk-2"}, everActive,
		"the tasks never became active, so clearing them proves nothing")
	assert.Empty(t, active, "a task never cleared: its terminal state did not arrive")

	// tk-1 is cleared by whichever terminal message arrives first — the stub
	// sends task_updated before task_notification, as the live run did. tk-2
	// gets no notification at all, so only task_updated can clear it.
	assert.Equal(t, "task_updated", clearedBy["tk-2"],
		"a task whose notification is suppressed must still clear on task_updated")
	assert.NotEmpty(t, clearedBy["tk-1"])
}

// The killed status is reported only by a task_updated patch, and it must reach
// the caller with the patch's error intact.
func TestE2E_KilledTaskReportsItsPatch(t *testing.T) {
	got := queryFakeCLI(t, "count the files")

	var killed *messages.TaskUpdatedMessage
	for i := range got {
		if tu := got[i].TaskUpdated; tu != nil && tu.Status == messages.TaskStatusKilled {
			killed = tu
		}
	}

	require.NotNil(t, killed, "the killed task_updated never reached the caller")
	assert.Equal(t, "tk-2", killed.TaskID)
	assert.True(t, killed.Status.IsTerminal())
	require.NotNil(t, killed.Patch.Error)
	assert.Equal(t, "stopped by TaskStop", *killed.Patch.Error)
	require.NotNil(t, killed.Patch.EndTime)
	assert.Equal(t, int64(1790295642000), *killed.Patch.EndTime)
}

func TestE2E_HookEventsReachTheCaller(t *testing.T) {
	got := queryFakeCLI(t, "count the files")

	var phases []messages.HookPhase
	var response *messages.HookEventMessage
	for i := range got {
		h := got[i].HookEvent
		if h == nil {
			continue
		}
		phases = append(phases, h.Phase)
		if h.Phase == messages.HookPhaseResponse {
			response = h
		}
	}

	// All three phases, including hook_progress — the one upstream's Python
	// SDK does not model, and which was unit-tested only until now.
	assert.Equal(t, []messages.HookPhase{
		messages.HookPhaseStarted,
		messages.HookPhaseProgress,
		messages.HookPhaseResponse,
	}, phases)

	require.NotNil(t, response)
	assert.Equal(t, "SessionStart:startup", response.HookName)
	assert.Equal(t, "SessionStart", response.HookEventName)
	assert.Equal(t, messages.HookOutcomeSuccess, response.Outcome)
	require.NotNil(t, response.ExitCode)
	assert.Equal(t, 0, *response.ExitCode)

	// Three distinct wire fields, preserved across the transport.
	assert.Equal(t, "on stdout", response.Stdout)
	assert.Equal(t, "on stderr", response.Stderr)
	assert.Equal(t, "the output", response.Output)
}

// Backward compatibility, proved over the wire rather than in a unit test: a
// caller written before this change reads every lifecycle event through System
// exactly as it did before, and Raw gives it the payload Data never carried.
func TestE2E_LifecycleEventsStillArriveAsSystemMessages(t *testing.T) {
	got := queryFakeCLI(t, "count the files")

	var subtypes []string
	for i := range got {
		if got[i].System != nil {
			subtypes = append(subtypes, got[i].System.Subtype)
			assert.NotEmpty(t, got[i].System.Raw,
				"System.Raw is empty for %s", got[i].System.Subtype)
			// Reading the deprecated field on purpose: that it stays empty is
			// the claim the deprecation rests on.
			//lint:ignore SA1019 asserting the deprecated field is empty is the test.
			assert.Nil(t, got[i].System.Data,
				"no CLI up to 2.1.267 emits a `data` key on a system message")
		}
	}

	// A subsequence rather than the exact list, so adding a frame to the stub
	// for some unrelated test does not break this one.
	assert.Subset(t, subtypes, []string{
		"hook_started",
		"hook_progress",
		"hook_response",
		"task_started",
		"task_progress",
		"task_updated",
		"task_notification",
		"background_tasks_changed",
	})
}

// An unmodelled subtype keeps reaching the caller with its payload intact. The
// content-block slice established this rule one level down; the same failure
// one level up would silently drop a message rather than a block.
func TestE2E_UnmodelledSubtypeSurvivesTheTransport(t *testing.T) {
	got := queryFakeCLI(t, "count the files")

	var raw string
	for i := range got {
		if got[i].System != nil && got[i].System.Subtype == "background_tasks_changed" {
			raw = string(got[i].System.Raw)
			assert.Nil(t, got[i].TaskStarted)
			assert.Nil(t, got[i].TaskUpdated)
			assert.Nil(t, got[i].HookEvent)
		}
	}

	require.NotEmpty(t, raw, "the unmodelled subtype was dropped")
	assert.JSONEq(t,
		`{"type":"system","subtype":"background_tasks_changed","tasks":[],"uuid":"u-bg","session_id":"e2e"}`,
		raw)
}
