package agent_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
)

// When does a Query end? Before 0.6.0, at the first ResultMessage. But a
// background task that finishes after that result wakes the session for a
// follow-up turn, and the one-shot Query had already closed the CLI's stdin:
// the hook the CLI asks to run for it is never answered — upstream's #1088 and
// #1190. These drive the stub CLI over real pipes playing claude 2.1.288,
// which reports session state to the SDK in frames marked sdk_host_only.

func fakeOpts(scenario string, extra map[string]string) agent.Options {
	env := map[string]string{"FAKECLI_SCENARIO": scenario}
	for k, v := range extra {
		env[k] = v
	}
	return agent.Options{CLIPath: fakeCLIPath, Env: env}
}

func collect(t *testing.T, seq func(func(messages.Message, error) bool)) []messages.Message {
	t.Helper()
	var got []messages.Message
	for msg, err := range seq {
		require.NoError(t, err)
		got = append(got, msg)
	}
	return got
}

func results(got []messages.Message) []string {
	var out []string
	for _, m := range got {
		if m.Result != nil {
			out = append(out, m.Result.Result)
		}
	}
	return out
}

func TestE2E_OneShotQueryAnswersAHookAfterTheFirstResult(t *testing.T) {
	var hookCalls atomic.Int32
	opts := fakeOpts("followup", nil)
	opts.HookHandlers = map[string][]agent.HookMatcher{
		"SubagentStop": {{Handler: func(context.Context, string, json.RawMessage) (map[string]interface{}, error) {
			hookCalls.Add(1)
			return map[string]interface{}{}, nil
		}}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got := collect(t, agent.Query(ctx, "count the files", opts))

	assert.Equal(t, int32(1), hookCalls.Load(),
		"the hook the CLI asked for after the first result was never answered")
	assert.Equal(t, []string{"Go 1.26 is out.", "The background agent finished."}, results(got),
		"the follow-up turn must arrive inside the same Query")
}

// The frames the SDK asked for are the SDK's: the caller did not opt in to
// session state and must not start receiving it.
func TestE2E_HostOnlyStateNeverReachesTheCaller(t *testing.T) {
	got := queryFakeCLIWith(t, fakeOpts("", nil), "count the files")

	for _, m := range got {
		assert.Nil(t, m.SessionStateChanged, "an sdk_host_only frame reached the caller")
		if m.System != nil {
			assert.NotEqual(t, "session_state_changed", m.System.Subtype, "an sdk_host_only frame reached the caller")
		}
	}
	require.NotEmpty(t, results(got))
}

// On a Client the follow-up turn used to be left queued and handed to the
// *next* Query, attributed to the wrong prompt.
func TestE2E_ClientQueryKeepsItsFollowUpTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	opts := fakeOpts("followup", nil)
	opts.HookHandlers = map[string][]agent.HookMatcher{
		"SubagentStop": {{Handler: func(context.Context, string, json.RawMessage) (map[string]interface{}, error) {
			return map[string]interface{}{}, nil
		}}},
	}
	client := agent.NewClient(opts)
	require.NoError(t, client.Connect(ctx))
	defer func() { _ = client.Disconnect() }()

	first := collect(t, client.Query(ctx, "count the files"))
	assert.Equal(t, []string{"Go 1.26 is out.", "The background agent finished."}, results(first))

	// The stub plays the same scenario for every prompt, so the second Query
	// must see exactly its own two results. Before, it opened with the first
	// Query's left-over follow-up result and stopped there.
	second := collect(t, client.Query(ctx, "again"))
	assert.Equal(t, []string{"Go 1.26 is out.", "The background agent finished."}, results(second),
		"the first Query's follow-up turn leaked into the second")
}

// A CLI that reports no session state (2.1.283 and older) gives nothing to
// wait for, so the result still ends the Query, as before.
func TestE2E_AnOldCLIStillEndsAtTheResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	got := collect(t, agent.Query(ctx, "count the files", fakeOpts("old-cli", nil)))

	assert.Equal(t, []string{"Go 1.26 is out."}, results(got))
	assert.Less(t, time.Since(start), 5*time.Second, "an old CLI's Query waited for an idle it never sends")
}

// State was reported but "idle" never comes. The wait is bounded by the same
// variable that bounds the CLI's own wait for background work.
func TestE2E_QueryStopsWaitingForIdleAtTheCeiling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	got := collect(t, agent.Query(ctx, "count the files",
		fakeOpts("noidle", map[string]string{"CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS": "300"})))
	elapsed := time.Since(start)

	assert.Equal(t, []string{"Go 1.26 is out."}, results(got))
	assert.GreaterOrEqual(t, elapsed, 300*time.Millisecond, "the Query ended before the ceiling; it did not wait for idle at all")
	assert.Less(t, elapsed, 10*time.Second, "the ceiling was not honoured")
}

// With the caller's own opt-in on a CLI that also honours the SDK's request,
// state arrives as ordinary frames, "idle" after the follow-up turn. The
// caller must receive that "idle", last, inside the Query. The first version
// of this decided the end from a notification that could overtake the frame
// itself, and against 2.1.288 the caller saw only "running".
func TestE2E_AnOptedInCallerSeesIdleLastInsideTheQuery(t *testing.T) {
	opts := fakeOpts("followup", map[string]string{"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS": "1"})
	opts.HookHandlers = map[string][]agent.HookMatcher{
		"SubagentStop": {{Handler: func(context.Context, string, json.RawMessage) (map[string]interface{}, error) {
			return map[string]interface{}{}, nil
		}}},
	}

	for i := 0; i < 20; i++ {
		got := queryFakeCLIWith(t, opts, "count the files")

		require.NotEmpty(t, got)
		last := got[len(got)-1]
		require.NotNil(t, last.SessionStateChanged, "run %d: the Query did not end on the caller's idle", i)
		assert.Equal(t, messages.SessionStateIdle, last.SessionStateChanged.State)
		assert.Equal(t, []string{"Go 1.26 is out.", "The background agent finished."}, results(got), "run %d", i)
	}
}
