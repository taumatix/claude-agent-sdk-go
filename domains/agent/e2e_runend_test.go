package agent_test

import (
	"context"
	"encoding/json"
	"path/filepath"
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

// 2.1.283 sends no session state unless the caller opts in, so for it the
// task frames are the only sign a turn is still owed. An agent started in the
// background and still running at the result will wake the session again;
// upstream keeps stdin open while one is in flight (DEFERRING_TASK_TYPES).
func TestE2E_AnOldCLIsQueryWaitsForABackgroundAgent(t *testing.T) {
	var hookCalls atomic.Int32
	opts := fakeOpts("old-cli-agent", nil)
	opts.HookHandlers = map[string][]agent.HookMatcher{
		"SubagentStop": {{Handler: func(context.Context, string, json.RawMessage) (map[string]interface{}, error) {
			hookCalls.Add(1)
			return map[string]interface{}{}, nil
		}}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got := collect(t, agent.Query(ctx, "review in the background", opts))

	assert.Equal(t, int32(1), hookCalls.Load(), "the background agent's hook was never answered")
	assert.Equal(t, []string{"Go 1.26 is out.", "The background agent finished."}, results(got))
}

// Only agents are waited for. A background shell can run for ever and may
// never report an end, so waiting on one would hang the Query.
func TestE2E_ABackgroundShellDoesNotHoldTheQueryOpen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	got := collect(t, agent.Query(ctx, "tail the log", fakeOpts("old-cli-shell", nil)))

	assert.Equal(t, []string{"Go 1.26 is out."}, results(got))
	assert.Less(t, time.Since(start), 5*time.Second, "a background shell held the Query open")
}

// An agent whose end never arrives is not cut off: the ceiling does not end
// the Query, because a cut-off agent loses stdin for its hooks (upstream's
// reasoning, and this SDK's choice). The caller's context is what bounds it,
// and it must still work.
func TestE2E_AnAgentWhoseEndIsLostHoldsTheQueryUntilTheCallersDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	start := time.Now()
	var got []messages.Message
	var last error
	for msg, err := range agent.Query(ctx, "review in the background",
		fakeOpts("old-cli-agent-lost", map[string]string{"CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS": "200"})) {
		if err != nil {
			last = err
			continue
		}
		got = append(got, msg)
	}
	elapsed := time.Since(start)

	assert.Equal(t, []string{"Go 1.26 is out."}, results(got))
	assert.ErrorIs(t, last, context.DeadlineExceeded, "the Query must end with the caller's own deadline")
	assert.GreaterOrEqual(t, elapsed, 1400*time.Millisecond, "the Query ended at the ceiling, cutting off an agent still in flight")
	assert.Less(t, elapsed, 10*time.Second, "the caller's deadline was not honoured")
}

// A lost agent does not hold the Client for good: the next background_tasks_changed
// the CLI sends lists what is really running, and an agent it leaves out is
// let go. Here that frame arrives with the next prompt's lifecycle frames.
func TestE2E_AClientRecoversFromALostAgentAtTheNextLevel(t *testing.T) {
	opts := fakeOpts("old-cli-agent-lost-once", map[string]string{
		"CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS": "200",
		"FAKECLI_STATE_FILE":                   filepath.Join(t.TempDir(), "lost"),
	})
	client := agent.NewClient(opts)
	bg := context.Background()
	require.NoError(t, client.Connect(bg))
	defer func() { _ = client.Disconnect() }()

	queryUntil := func(d time.Duration, prompt string) (results []string, err error) {
		ctx, cancel := context.WithTimeout(bg, d)
		defer cancel()
		for msg, e := range client.Query(ctx, prompt) {
			if e != nil {
				err = e
			} else if msg.Result != nil {
				results = append(results, msg.Result.Result)
			}
		}
		return results, err
	}

	_, err := queryUntil(500*time.Millisecond, "review in the background")
	require.ErrorIs(t, err, context.DeadlineExceeded, "the lost agent was let go before the CLI said so")

	got, err := queryUntil(10*time.Second, "again")
	require.NoError(t, err, "the lost agent still held the Client after the CLI stopped listing it")
	assert.Equal(t, []string{"Go 1.26 is out."}, got)
}

// The ledger is the CLI process's: the CLI sends nothing at start-up, so a new
// process has no agent in flight whatever the last one left. A Client that
// Disconnects and Connects again has a new process, and must not wait for an
// agent the old one lost. The stub's second process sends no level frame, so
// nothing but a fresh ledger lets the Query end.
func TestE2E_AReconnectedClientDoesNotWaitForTheOldProcessesAgent(t *testing.T) {
	opts := fakeOpts("old-cli-agent-lost-once", map[string]string{
		"FAKECLI_NO_LEVEL":                     "1",
		"CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS": "200",
		"FAKECLI_STATE_FILE":                   filepath.Join(t.TempDir(), "lost"),
	})
	client := agent.NewClient(opts)
	bg := context.Background()
	require.NoError(t, client.Connect(bg))
	defer func() { _ = client.Disconnect() }()

	first, cancel := context.WithTimeout(bg, 500*time.Millisecond)
	var held error
	for _, err := range client.Query(first, "review in the background") {
		if err != nil {
			held = err
		}
	}
	cancel()
	require.ErrorIs(t, held, context.DeadlineExceeded, "the first process's agent was not held; the test proves nothing")

	require.NoError(t, client.Disconnect())
	require.NoError(t, client.Connect(bg))

	second, cancel := context.WithTimeout(bg, 10*time.Second)
	defer cancel()
	var got []string
	for msg, err := range client.Query(second, "again") {
		require.NoError(t, err, "the new process was held by the old one's agent")
		if msg.Result != nil {
			got = append(got, msg.Result.Result)
		}
	}
	assert.Equal(t, []string{"Go 1.26 is out."}, got)
}

// "idle" arriving while an agent is still in flight does not end the Query:
// the agent's end will wake the session for another turn.
func TestE2E_IdleWithAnAgentInFlightDoesNotEndTheQuery(t *testing.T) {
	var hookCalls atomic.Int32
	opts := fakeOpts("idle-every-turn", nil)
	opts.HookHandlers = map[string][]agent.HookMatcher{
		"SubagentStop": {{Handler: func(context.Context, string, json.RawMessage) (map[string]interface{}, error) {
			hookCalls.Add(1)
			return map[string]interface{}{}, nil
		}}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got := collect(t, agent.Query(ctx, "review in the background", opts))

	assert.Equal(t, int32(1), hookCalls.Load(), "the agent's hook was never answered")
	assert.Equal(t, []string{"Go 1.26 is out.", "The background agent finished."}, results(got))
}

// A lost task_notification no longer holds the Query once the CLI's level
// signal, background_tasks_changed, stops listing the agent. Here the
// follow-up turn ends the Query; no context deadline is involved.
func TestE2E_TheLevelSignalReleasesAnAgentWhoseBookendWasLost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	got := collect(t, agent.Query(ctx, "review in the background", fakeOpts("old-cli-agent-bookend-lost", nil)))

	assert.Equal(t, []string{"Go 1.26 is out.", "The background agent finished."}, results(got))
	assert.Less(t, time.Since(start), 10*time.Second, "the Query waited for a bookend the level had made unnecessary")
}

// The level must not release an agent it still lists, nor count a shell.
func TestE2E_TheLevelSignalKeepsAnAgentItStillLists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	var last error
	for _, err := range agent.Query(ctx, "review in the background",
		fakeOpts("old-cli-agent-level-stale", map[string]string{"CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS": "200"})) {
		if err != nil {
			last = err
		}
	}
	assert.ErrorIs(t, last, context.DeadlineExceeded, "an agent the level still listed was let go")
}

func TestE2E_APermissionDenialArrivesTyped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var denied []*messages.PermissionDeniedMessage
	for _, m := range collect(t, agent.Query(ctx, "run echo hi", fakeOpts("old-cli-permission-denied", nil))) {
		if m.PermissionDenied != nil {
			denied = append(denied, m.PermissionDenied)
		}
	}
	require.Len(t, denied, 1)
	assert.Equal(t, "Bash", denied[0].ToolName)
	assert.Equal(t, "subcommandResults", denied[0].ReasonType)
}

func TestE2E_AnAPIRetryArrivesTyped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var retries []*messages.APIRetryMessage
	for _, m := range collect(t, agent.Query(ctx, "hi", fakeOpts("old-cli-api-retry", nil))) {
		if m.APIRetry != nil {
			retries = append(retries, m.APIRetry)
		}
	}
	require.Len(t, retries, 1)
	assert.Equal(t, 529, retries[0].ErrorStatus)
	assert.Equal(t, "overloaded", retries[0].Error)
}

// The stub's lifecycle frames, played on every prompt, include the
// thinking_tokens frame a 2026-09-27 live run sent.
func TestE2E_ThinkingProgressArrivesTyped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var thinking []*messages.ThinkingTokensMessage
	for _, m := range collect(t, agent.Query(ctx, "hi", fakeOpts("old-cli", nil))) {
		if m.ThinkingTokens != nil {
			thinking = append(thinking, m.ThinkingTokens)
		}
	}
	require.Len(t, thinking, 1)
	assert.Equal(t, 50, thinking[0].EstimatedTokens)
	assert.Equal(t, 50, thinking[0].Delta)
}

// The stub plays the compact_boundary frame a live 2.1.288 /compact sent.
func TestE2E_CompactionArrivesTyped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var compacted []*messages.CompactBoundaryMessage
	for _, m := range collect(t, agent.Query(ctx, "hi", fakeOpts("old-cli", nil))) {
		if m.CompactBoundary != nil {
			require.NotNil(t, m.System, "a typed compaction must still arrive as System")
			compacted = append(compacted, m.CompactBoundary)
		}
	}
	require.Len(t, compacted, 1)
	assert.Equal(t, "manual", compacted[0].Trigger)
	assert.Equal(t, 33919, compacted[0].PreTokens)
	assert.Equal(t, 3337, compacted[0].PostTokens)
	assert.Equal(t, 12564*time.Millisecond, compacted[0].Duration)
}

// The stub plays an informational frame built from the 2.1.288 schema; no live
// CLI has been seen to send one, so this proves the typing, not the wire.
func TestE2E_InformationalArrivesTyped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var notes []*messages.InformationalMessage
	for _, m := range collect(t, agent.Query(ctx, "hi", fakeOpts("old-cli", nil))) {
		if m.Informational != nil {
			require.NotNil(t, m.System, "a typed notice must still arrive as System")
			notes = append(notes, m.Informational)
		}
	}
	require.Len(t, notes, 1)
	assert.Equal(t, "Context is 90% full", notes[0].Content)
	assert.Equal(t, "warning", notes[0].Level)
	assert.False(t, notes[0].PreventContinuation)
}
