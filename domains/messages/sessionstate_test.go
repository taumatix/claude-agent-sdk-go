package messages_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
)

// Frames a real `claude` 2.1.283 run emitted on 2026-09-27 with
// CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1 while running one background Bash
// command, with session ids shortened. Field names and nesting are verbatim and
// agree with the zod schemas the CLI bundles for both subtypes.
const (
	liveSessionRunning = `{"type":"system","subtype":"session_state_changed","state":"running",` +
		`"uuid":"u-running","session_id":"sess-2"}`

	liveSessionIdle = `{"type":"system","subtype":"session_state_changed","state":"idle",` +
		`"uuid":"u-idle","session_id":"sess-2"}`

	liveBackgroundOne = `{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"bmrd57x9h",` +
		`"task_type":"local_bash","description":"Wait three seconds then print done"}],` +
		`"uuid":"u-bg-1","session_id":"sess-2"}`

	liveBackgroundNone = `{"type":"system","subtype":"background_tasks_changed","tasks":[],` +
		`"uuid":"u-bg-0","session_id":"sess-2"}`
)

func TestSessionStateChangedIsTyped(t *testing.T) {
	for _, tc := range []struct {
		line  string
		state messages.SessionState
		uuid  string
	}{
		{liveSessionRunning, messages.SessionStateRunning, "u-running"},
		{liveSessionIdle, messages.SessionStateIdle, "u-idle"},
	} {
		msg := convert(t, tc.line)

		require.NotNil(t, msg.SessionStateChanged)
		s := msg.SessionStateChanged
		assert.Equal(t, tc.state, s.State)
		assert.Equal(t, tc.uuid, s.UUID)
		assert.Equal(t, "sess-2", s.SessionID)
		assert.Nil(t, s.WaitingOnUser, "the SDK-facing emitter does not send waiting_on_user")
	}
}

// requires_action is declared by the schema but was not provoked by the live
// run; the literal is the schema's own enum value.
func TestSessionStateRequiresAction(t *testing.T) {
	msg := convert(t, `{"type":"system","subtype":"session_state_changed","state":"requires_action",`+
		`"uuid":"u","session_id":"s"}`)

	require.NotNil(t, msg.SessionStateChanged)
	assert.Equal(t, messages.SessionStateRequiresAction, msg.SessionStateChanged.State)
}

// The bridge emitter adds waiting_on_user; it is decoded when present.
func TestSessionStateCarriesWaitingOnUser(t *testing.T) {
	msg := convert(t, `{"type":"system","subtype":"session_state_changed","state":"running",`+
		`"waiting_on_user":true,"uuid":"u","session_id":"s"}`)

	require.NotNil(t, msg.SessionStateChanged)
	require.NotNil(t, msg.SessionStateChanged.WaitingOnUser)
	assert.True(t, *msg.SessionStateChanged.WaitingOnUser)
}

// A state this SDK does not know is passed through rather than dropped: the
// caller decides what an unfamiliar state means.
func TestSessionStateUnknownValueIsKept(t *testing.T) {
	msg := convert(t, `{"type":"system","subtype":"session_state_changed","state":"hibernating",`+
		`"uuid":"u","session_id":"s"}`)

	require.NotNil(t, msg.SessionStateChanged)
	assert.Equal(t, messages.SessionState("hibernating"), msg.SessionStateChanged.State)
}

func TestBackgroundTasksChangedIsTyped(t *testing.T) {
	msg := convert(t, liveBackgroundOne)

	require.NotNil(t, msg.BackgroundTasksChanged)
	b := msg.BackgroundTasksChanged
	assert.Equal(t, "u-bg-1", b.UUID)
	assert.Equal(t, "sess-2", b.SessionID)
	require.Len(t, b.Tasks, 1)
	assert.Equal(t, messages.BackgroundTask{
		TaskID:      "bmrd57x9h",
		TaskType:    "local_bash",
		Description: "Wait three seconds then print done",
	}, b.Tasks[0])
}

// The empty set is the payload that says background work finished. It must
// arrive typed, and as a non-nil empty slice, so a caller replacing its set
// cannot confuse it with "no set was sent".
func TestBackgroundTasksChangedEmptySetIsTyped(t *testing.T) {
	msg := convert(t, liveBackgroundNone)

	require.NotNil(t, msg.BackgroundTasksChanged)
	assert.NotNil(t, msg.BackgroundTasksChanged.Tasks)
	assert.Empty(t, msg.BackgroundTasksChanged.Tasks)
}

func TestBackgroundTaskDecodesAmbient(t *testing.T) {
	msg := convert(t, `{"type":"system","subtype":"background_tasks_changed","tasks":[`+
		`{"task_id":"t1","task_type":"local_agent","description":"d","ambient":true}],"uuid":"u","session_id":"s"}`)

	require.NotNil(t, msg.BackgroundTasksChanged)
	require.Len(t, msg.BackgroundTasksChanged.Tasks, 1)
	assert.True(t, msg.BackgroundTasksChanged.Tasks[0].Ambient)
}

// The payload has REPLACE semantics. A frame with no `tasks` key typed as an
// empty set would tell every caller that all background work stopped, so it
// degrades to System instead.
func TestBackgroundTasksChangedWithoutTasksKeyDegradesToSystem(t *testing.T) {
	testMalformedPayloadDegradesToSystem(t, "background_tasks_changed",
		`{"type":"system","subtype":"background_tasks_changed","uuid":"u","session_id":"s"}`)

	msg := convert(t, `{"type":"system","subtype":"background_tasks_changed","tasks":null,"uuid":"u","session_id":"s"}`)
	assert.Nil(t, msg.BackgroundTasksChanged, "a null set is not an empty set")
}

func TestMalformedSessionPayloadsDegradeToSystem(t *testing.T) {
	testMalformedPayloadDegradesToSystem(t, "session_state_changed",
		`{"type":"system","subtype":"session_state_changed","state":7,"uuid":"u","session_id":"s"}`)
	testMalformedPayloadDegradesToSystem(t, "session_state_changed",
		`{"type":"system","subtype":"session_state_changed","uuid":"u","session_id":"s"}`)
	testMalformedPayloadDegradesToSystem(t, "background_tasks_changed",
		`{"type":"system","subtype":"background_tasks_changed","tasks":{"task_id":"x"},"uuid":"u","session_id":"s"}`)
}

func TestSessionAndBackgroundMessagesStillArriveAsSystem(t *testing.T) {
	for _, line := range []string{liveSessionRunning, liveBackgroundOne} {
		msg := convert(t, line)
		require.NotNil(t, msg.System)
		assert.NotEmpty(t, msg.System.Raw)
	}
}
