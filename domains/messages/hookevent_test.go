package messages_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
)

// liveHookStarted and liveHookResponse are frames from the same real `claude`
// 2.1.267 run as the task fixtures, with the hook's output replaced (it was
// this machine's own hook text) and ids shortened. The field set is verbatim.
//
// liveHookProgress is schema-derived, not observed: the run's hooks finished
// before the CLI's ~1s progress timer fired. Its shape is the zod schema the
// 2.1.267 bundle carries for the subtype.
const (
	liveHookStarted = `{"type":"system","subtype":"hook_started","hook_id":"hk-1",` +
		`"hook_name":"SessionStart:startup","hook_event":"SessionStart",` +
		`"uuid":"u-hs","session_id":"sess-1"}`

	// stdout, stderr and output are given three different values on purpose.
	// The real capture had stdout == output, which made sourcing Stdout from
	// the wrong field an undetectable mistake: the assertion passed either way.
	liveHookProgress = `{"type":"system","subtype":"hook_progress","hook_id":"hk-1",` +
		`"hook_name":"SessionStart:startup","hook_event":"SessionStart",` +
		`"stdout":"on stdout","stderr":"on stderr","output":"the output",` +
		`"uuid":"u-hp","session_id":"sess-1"}`

	liveHookResponse = `{"type":"system","subtype":"hook_response","hook_id":"hk-1",` +
		`"hook_name":"SessionStart:startup","hook_event":"SessionStart",` +
		`"output":"the output","stdout":"on stdout","stderr":"on stderr",` +
		`"exit_code":0,"outcome":"success","uuid":"u-hr","session_id":"sess-1"}`
)

func TestHookStartedIsTyped(t *testing.T) {
	msg := convert(t, liveHookStarted)

	require.NotNil(t, msg.HookEvent)
	h := msg.HookEvent
	assert.Equal(t, messages.HookPhaseStarted, h.Phase)
	assert.Equal(t, "hk-1", h.HookID)
	assert.Equal(t, "sess-1", h.SessionID)
	assert.Equal(t, "u-hs", h.UUID)

	// hook_name and hook_event are different fields. Upstream collapses them
	// with `data.get("hook_event") or data.get("hook_name") or ...`; the live
	// capture shows the hook's name embeds the event without equalling it.
	assert.Equal(t, "SessionStart:startup", h.HookName)
	assert.Equal(t, "SessionStart", h.HookEventName)
	assert.NotEqual(t, h.HookName, h.HookEventName)

	// Only hook_response carries these.
	assert.Nil(t, h.ExitCode)
	assert.Equal(t, messages.HookOutcome(""), h.Outcome)
}

// hook_progress is a third phase upstream's Python SDK does not model at all:
// its parser routes only hook_started and hook_response. The CLI emits this one
// from an interval timer while a hook runs.
func TestHookProgressIsTyped(t *testing.T) {
	msg := convert(t, liveHookProgress)

	require.NotNil(t, msg.HookEvent)
	h := msg.HookEvent
	assert.Equal(t, messages.HookPhaseProgress, h.Phase)
	assert.Equal(t, "the output", h.Output)
	assert.Equal(t, "on stdout", h.Stdout)
	assert.Equal(t, "on stderr", h.Stderr)
	assert.Nil(t, h.ExitCode, "exit_code arrives only once the hook has finished")
}

func TestHookResponseCarriesOutcomeAndExitCode(t *testing.T) {
	msg := convert(t, liveHookResponse)

	require.NotNil(t, msg.HookEvent)
	h := msg.HookEvent
	assert.Equal(t, messages.HookPhaseResponse, h.Phase)
	assert.Equal(t, messages.HookOutcomeSuccess, h.Outcome)

	// Three separate wire fields, three separate values.
	assert.Equal(t, "the output", h.Output)
	assert.Equal(t, "on stdout", h.Stdout)
	assert.Equal(t, "on stderr", h.Stderr)

	// A successful hook exits 0, so a plain int would make "exited 0" and
	// "did not report an exit code" the same value. It is a pointer for that
	// reason and the distinction is worth holding onto.
	require.NotNil(t, h.ExitCode)
	assert.Equal(t, 0, *h.ExitCode)
}

func TestHookResponseExitCodeAbsentStaysNil(t *testing.T) {
	msg := convert(t, `{"type":"system","subtype":"hook_response","hook_id":"hk-2",`+
		`"hook_name":"PreToolUse:guard","hook_event":"PreToolUse","output":"denied",`+
		`"stdout":"","stderr":"blocked","outcome":"error","uuid":"u","session_id":"s"}`)

	require.NotNil(t, msg.HookEvent)
	assert.Nil(t, msg.HookEvent.ExitCode)
	assert.Equal(t, messages.HookOutcomeError, msg.HookEvent.Outcome)
	assert.Equal(t, "blocked", msg.HookEvent.Stderr)
}

func TestHookOutcomeCancelled(t *testing.T) {
	msg := convert(t, `{"type":"system","subtype":"hook_response","hook_id":"hk-3",`+
		`"hook_name":"Stop:cleanup","hook_event":"Stop","outcome":"cancelled",`+
		`"exit_code":143,"uuid":"u","session_id":"s"}`)

	require.NotNil(t, msg.HookEvent)
	assert.Equal(t, messages.HookOutcomeCancelled, msg.HookEvent.Outcome)
	require.NotNil(t, msg.HookEvent.ExitCode)
	assert.Equal(t, 143, *msg.HookEvent.ExitCode)
}

// All three phases keep arriving as System too, so existing code is unaffected.
func TestHookEventsStillArriveAsSystem(t *testing.T) {
	for _, line := range []string{liveHookStarted, liveHookProgress, liveHookResponse} {
		msg := convert(t, line)
		require.NotNil(t, msg.System)
		assert.Equal(t, "sess-1", msg.System.SessionID)
		assert.JSONEq(t, line, string(msg.System.Raw))
		// A hook event is not a task event; the task fields stay nil.
		assert.Nil(t, msg.TaskStarted)
		assert.Nil(t, msg.TaskUpdated)
	}
}

// The Phase constants are the wire subtypes, so a caller can compare either.
func TestHookPhaseMatchesTheSystemSubtype(t *testing.T) {
	for _, line := range []string{liveHookStarted, liveHookProgress, liveHookResponse} {
		msg := convert(t, line)
		require.NotNil(t, msg.HookEvent)
		require.NotNil(t, msg.System)
		assert.Equal(t, msg.System.Subtype, string(msg.HookEvent.Phase))
	}
}
