package agent

import (
	"context"
	"iter"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
	subprocess "github.com/taumatix/claude-agent-sdk-go/domains/transport/subprocess"
)

// Query executes a one-shot prompt and returns a range iterator over response messages.
//
// Usage:
//
//	for msg, err := range agent.Query(ctx, "What is 2+2?", agent.DefaultOptions()) {
//	    if err != nil { log.Fatal(err) }
//	    if msg.Assistant != nil { /* process assistant message */ }
//	    if msg.Result != nil { /* a turn ended; another may follow */ }
//	}
//
// The iterator ends when the run is over, which is not always the first
// [messages.ResultMessage]: a background agent that finishes after a result
// wakes the session for a follow-up turn, with its own result. Breaking out at
// the first result abandons that turn and leaves its hooks unanswered.
//
// A background agent the CLI reported starting holds the Query open until the
// CLI reports it finished. The CLI's wait ceiling (CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS)
// does not cut it off, because a cut-off agent loses stdin for its hooks and
// permission requests; upstream's Python SDK makes the same choice. So if that
// finish never arrives, neither as a task_notification nor as a
// background_tasks_changed that stops listing it (the CLI crashed, or a later
// CLI renamed both frames), the Query never ends on its own. **ctx is the
// bound:** give it a deadline, and the Query ends with ctx's error when it
// passes.
func Query(ctx context.Context, prompt string, opts Options) iter.Seq2[messages.Message, error] {
	return func(yield func(messages.Message, error) bool) {
		t := opts.Transport
		if t == nil {
			st, err := subprocess.New(ctx, subprocess.Config{
				CLIPath:          opts.CLIPath,
				Args:             BuildCLIArgs(opts),
				Env:              subprocessEnv(opts.Env),
				WorkingDirectory: opts.WorkingDirectory,
			})
			if err != nil {
				yield(messages.Message{}, err)
				return
			}
			t = st
		}

		sm := newSessionManager(ctx, t, opts)
		defer sm.Close()

		if err := sm.initialize(); err != nil {
			yield(messages.Message{}, err)
			return
		}

		if err := sm.sendUserMessage(prompt); err != nil {
			yield(messages.Message{}, err)
			return
		}

		// Not at the first result: the CLI may still owe a follow-up turn,
		// and the deferred Close shuts its stdin. See run.
		sm.run(ctx, yield)
	}
}
