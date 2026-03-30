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
//	    if msg.Result != nil { return } // final message
//	}
func Query(ctx context.Context, prompt string, opts Options) iter.Seq2[messages.Message, error] {
	return func(yield func(messages.Message, error) bool) {
		t := opts.Transport
		if t == nil {
			st, err := subprocess.New(ctx, subprocess.Config{
				CLIPath:          opts.CLIPath,
				Args:             BuildCLIArgs(opts),
				Env:              opts.Env,
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

		for {
			select {
			case msg, ok := <-sm.Messages():
				if !ok {
					// msgCh closed — drain errCh for any terminal error
					select {
					case err := <-sm.Errors():
						if err != nil {
							yield(messages.Message{}, err)
						}
					default:
					}
					return
				}
				if !yield(msg, nil) {
					return
				}
				if msg.Result != nil {
					return
				}
			case err := <-sm.Errors():
				if err != nil {
					yield(messages.Message{}, err)
				}
				return
			case <-ctx.Done():
				yield(messages.Message{}, ctx.Err())
				return
			}
		}
	}
}
