package agent

import (
	"context"
	"iter"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
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
		c := NewClient(opts)
		if err := c.Connect(ctx); err != nil {
			yield(messages.Message{}, err)
			return
		}
		defer c.Disconnect()
		for msg, err := range c.Query(ctx, prompt) {
			if !yield(msg, err) {
				return
			}
		}
	}
}
