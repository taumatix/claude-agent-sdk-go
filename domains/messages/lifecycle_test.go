package messages_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

// The fixtures below are the frames a real `claude` 2.1.267 run emitted on
// 2026-09-25 while spawning an Explore subagent, with session ids and the
// subagent prompt shortened. Field names and nesting are verbatim; they agree
// with the zod schemas the CLI bundles for each subtype. Asserting the literal
// values the CLI produced is the point — a fixture this SDK invented would only
// prove the parser agrees with itself.
const (
	liveTaskStarted = `{"type":"system","subtype":"task_started","task_id":"a568bf31104001c8d",` +
		`"tool_use_id":"toolu_01PrysQffLqBvZ6wTiTvoaza","description":"Count files in /tmp/taskprobe",` +
		`"subagent_type":"Explore","is_backgrounded":false,"spawn_depth":1,"task_type":"local_agent",` +
		`"prompt":"Count how many files are in /tmp/taskprobe.","uuid":"u-started","session_id":"sess-1"}`

	liveTaskProgress = `{"type":"system","subtype":"task_progress","task_id":"a568bf31104001c8d",` +
		`"tool_use_id":"toolu_01PrysQffLqBvZ6wTiTvoaza","description":"Running List and count files",` +
		`"subagent_type":"Explore","usage":{"total_tokens":12815,"tool_uses":1,"duration_ms":2200},` +
		`"last_tool_name":"Bash","uuid":"u-progress","session_id":"sess-1"}`

	liveTaskUpdated = `{"type":"system","subtype":"task_updated","task_id":"a568bf31104001c8d",` +
		`"patch":{"status":"completed","end_time":1790295641558},"uuid":"u-updated","session_id":"sess-1"}`

	liveTaskNotification = `{"type":"system","subtype":"task_notification","task_id":"a568bf31104001c8d",` +
		`"tool_use_id":"toolu_01PrysQffLqBvZ6wTiTvoaza","status":"completed",` +
		`"output_file":"/tmp/claude-504/tasks/a568bf31104001c8d.output","summary":"Non-recursive count: 3 regular files.",` +
		`"usage":{"total_tokens":12815,"tool_uses":1,"duration_ms":2200},"uuid":"u-notification","session_id":"sess-1"}`
)

func convert(t *testing.T, line string) *messages.Message {
	t.Helper()
	w, err := protocol.ParseLine([]byte(line))
	require.NoError(t, err)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg)
	return msg
}

func TestTaskStartedIsTyped(t *testing.T) {
	msg := convert(t, liveTaskStarted)

	require.NotNil(t, msg.TaskStarted)
	ts := msg.TaskStarted
	assert.Equal(t, "a568bf31104001c8d", ts.TaskID)
	assert.Equal(t, "toolu_01PrysQffLqBvZ6wTiTvoaza", ts.ToolUseID)
	assert.Equal(t, "Count files in /tmp/taskprobe", ts.Description)
	assert.Equal(t, "Explore", ts.SubagentType)
	assert.Equal(t, "local_agent", ts.TaskType)
	assert.Equal(t, "Count how many files are in /tmp/taskprobe.", ts.Prompt)
	assert.Equal(t, "u-started", ts.UUID)
	assert.Equal(t, "sess-1", ts.SessionID)

	// Tri-state: the CLI sets is_backgrounded only for local_agent/local_bash
	// tasks, so "absent" and "false" are different answers.
	require.NotNil(t, ts.IsBackgrounded)
	assert.False(t, *ts.IsBackgrounded)
	require.NotNil(t, ts.SpawnDepth)
	assert.Equal(t, 1, *ts.SpawnDepth)

	// Absent in the capture; the schema's own wording makes absence false.
	assert.False(t, ts.Ambient)
	assert.False(t, ts.SkipTranscript)
}

func TestTaskProgressIsTyped(t *testing.T) {
	msg := convert(t, liveTaskProgress)

	require.NotNil(t, msg.TaskProgress)
	tp := msg.TaskProgress
	assert.Equal(t, "a568bf31104001c8d", tp.TaskID)
	assert.Equal(t, "Explore", tp.SubagentType)
	assert.Equal(t, "Bash", tp.LastToolName)
	assert.Equal(t, 12815, tp.Usage.TotalTokens)
	assert.Equal(t, 1, tp.Usage.ToolUses)
	assert.Equal(t, int64(2200), tp.Usage.DurationMS)
}

func TestTaskUpdatedCarriesThePatchAndItsTerminalStatus(t *testing.T) {
	msg := convert(t, liveTaskUpdated)

	require.NotNil(t, msg.TaskUpdated)
	tu := msg.TaskUpdated
	assert.Equal(t, "a568bf31104001c8d", tu.TaskID)
	assert.Equal(t, messages.TaskStatusCompleted, tu.Status)
	assert.True(t, tu.Status.IsTerminal())

	require.NotNil(t, tu.Patch.Status)
	assert.Equal(t, messages.TaskStatusCompleted, *tu.Patch.Status)
	require.NotNil(t, tu.Patch.EndTime)
	assert.Equal(t, int64(1790295641558), *tu.Patch.EndTime)
	assert.Nil(t, tu.Patch.Error)

	// The full patch stays reachable: the CLI may add fields to it, and a
	// caller should not have to wait for an SDK release to read one.
	assert.JSONEq(t, `{"status":"completed","end_time":1790295641558}`, string(tu.RawPatch))
}

func TestTaskNotificationIsTyped(t *testing.T) {
	msg := convert(t, liveTaskNotification)

	require.NotNil(t, msg.TaskNotification)
	tn := msg.TaskNotification
	assert.Equal(t, "a568bf31104001c8d", tn.TaskID)
	assert.Equal(t, messages.TaskStatusCompleted, tn.Status)
	assert.Equal(t, "/tmp/claude-504/tasks/a568bf31104001c8d.output", tn.OutputFile)
	assert.Equal(t, "Non-recursive count: 3 regular files.", tn.Summary)
	require.NotNil(t, tn.Usage)
	assert.Equal(t, 12815, tn.Usage.TotalTokens)
}

// A task that never emits a task_notification reports its terminal state only
// through a task_updated patch — the case the entry was written for.
func TestTaskUpdatedReportsKilledAsTerminal(t *testing.T) {
	msg := convert(t, `{"type":"system","subtype":"task_updated","task_id":"t9",`+
		`"patch":{"status":"killed","end_time":17,"error":"stopped by TaskStop"},"uuid":"u","session_id":"s"}`)

	require.NotNil(t, msg.TaskUpdated)
	assert.Equal(t, messages.TaskStatusKilled, msg.TaskUpdated.Status)
	assert.True(t, msg.TaskUpdated.Status.IsTerminal())
	require.NotNil(t, msg.TaskUpdated.Patch.Error)
	assert.Equal(t, "stopped by TaskStop", *msg.TaskUpdated.Patch.Error)
}

// task_updated and task_notification name the same transition differently:
// the CLI maps the raw "killed" to "stopped" only when it notifies. Both are
// terminal, and IsTerminal exists so a caller need not know that.
func TestTerminalStatusesSpanBothVocabularies(t *testing.T) {
	for _, s := range []messages.TaskStatus{
		messages.TaskStatusCompleted,
		messages.TaskStatusFailed,
		messages.TaskStatusKilled,
		messages.TaskStatusStopped,
	} {
		assert.True(t, s.IsTerminal(), "%s should be terminal", s)
	}
	for _, s := range []messages.TaskStatus{
		messages.TaskStatusPending,
		messages.TaskStatusRunning,
		messages.TaskStatusPaused,
		messages.TaskStatus(""),
		messages.TaskStatus("some_status_a_later_cli_invented"),
	} {
		assert.False(t, s.IsTerminal(), "%s should not be terminal", s)
	}
}

// A patch carrying no status leaves Status empty rather than guessing, and
// non-terminal rather than terminal — the safe direction for a caller clearing
// active-task state.
func TestTaskUpdatedWithNoStatusInPatchIsNotTerminal(t *testing.T) {
	msg := convert(t, `{"type":"system","subtype":"task_updated","task_id":"t9",`+
		`"patch":{"description":"still going"},"uuid":"u","session_id":"s"}`)

	require.NotNil(t, msg.TaskUpdated)
	assert.Equal(t, messages.TaskStatus(""), msg.TaskUpdated.Status)
	assert.False(t, msg.TaskUpdated.Status.IsTerminal())
	assert.Nil(t, msg.TaskUpdated.Patch.Status)
}

// Backward compatibility: code written against Message.System keeps working.
// This is the Go stand-in for upstream making these dataclasses subclasses of
// SystemMessage so `isinstance(msg, SystemMessage)` keeps matching.
func TestTypedLifecycleMessagesStillArriveAsSystem(t *testing.T) {
	for _, line := range []string{liveTaskStarted, liveTaskProgress, liveTaskUpdated, liveTaskNotification} {
		msg := convert(t, line)
		require.NotNil(t, msg.System, "System must stay populated for %s", line[:60])
		assert.Equal(t, "sess-1", msg.System.SessionID)
		assert.NotEmpty(t, msg.System.Subtype)
	}
}

// Data has been bound to a `data` key that no CLI emits since the port was
// written. Raw is where the payload actually lives.
func TestSystemMessageRawCarriesTheWholeMessage(t *testing.T) {
	msg := convert(t, liveTaskUpdated)

	require.NotNil(t, msg.System)
	assert.Nil(t, msg.System.Data, "no system frame from CLI 2.1.267 carries a `data` key")
	assert.JSONEq(t, liveTaskUpdated, string(msg.System.Raw))

	// Raw is what makes an unmodelled field reachable without an SDK release.
	var probe struct {
		Patch struct {
			EndTime int64 `json:"end_time"`
		} `json:"patch"`
	}
	require.NoError(t, json.Unmarshal(msg.System.Raw, &probe))
	assert.Equal(t, int64(1790295641558), probe.Patch.EndTime)
}

// Forward compatibility: a subtype this SDK does not model keeps arriving as a
// generic System message with its payload intact, rather than being dropped.
func TestUnmodelledSubtypeStillArrivesAsSystem(t *testing.T) {
	line := `{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"t1"}],"uuid":"u","session_id":"s"}`
	msg := convert(t, line)

	require.NotNil(t, msg.System)
	assert.Equal(t, "background_tasks_changed", msg.System.Subtype)
	assert.Nil(t, msg.TaskStarted)
	assert.Nil(t, msg.TaskUpdated)
	assert.JSONEq(t, line, string(msg.System.Raw))
}

// A lifecycle event must never fail the stream. Upstream raises
// MessageParseError when task_started is missing a key, which lets a progress
// notification kill a run; upstream's own task_updated branch says parsing
// "must never raise on a lifecycle event". That rule is applied to all of them.
func TestMalformedLifecyclePayloadDegradesToSystem(t *testing.T) {
	// `patch` is an array where the schema says object — a shape no amount of
	// defensive field-reading can rescue.
	line := `{"type":"system","subtype":"task_updated","task_id":"t1","patch":[1,2,3],"uuid":"u","session_id":"s"}`

	w, err := protocol.ParseLine([]byte(line))
	require.NoError(t, err)
	msg, err := messages.FromWire(w)
	require.NoError(t, err, "a malformed lifecycle event must not fail the stream")
	require.NotNil(t, msg)

	assert.Nil(t, msg.TaskUpdated, "an undecodable payload must not be handed over half-filled")
	require.NotNil(t, msg.System)
	assert.Equal(t, "task_updated", msg.System.Subtype)
	assert.JSONEq(t, line, string(msg.System.Raw))
}
