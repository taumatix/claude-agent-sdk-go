package agent

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
	"github.com/taumatix/claude-agent-sdk-go/domains/transport"
	"github.com/taumatix/claude-agent-sdk-go/domains/transport/subprocess"
)

// Client is a stateful agent client. Connect once, then call Query multiple times.
type Client struct {
	opts      Options
	sm        *sessionManager
	transport transport.Transport
	mu        sync.Mutex
}

// NewClient creates a new Client with the given options.
func NewClient(opts Options) *Client {
	return &Client{opts: opts}
}

// Connect starts the subprocess and performs the initialization handshake.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sm != nil {
		return fmt.Errorf("already connected")
	}
	t := c.opts.Transport
	if t == nil {
		st, err := subprocess.New(ctx, subprocess.Config{
			CLIPath:          c.opts.CLIPath,
			Args:             BuildCLIArgs(c.opts),
			Env:              c.opts.Env,
			WorkingDirectory: c.opts.WorkingDirectory,
		})
		if err != nil {
			return err
		}
		t = st
	}
	c.transport = t
	c.sm = newSessionManager(ctx, t, c.opts)
	return c.sm.initialize()
}

// Disconnect shuts down the session and transport.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sm == nil {
		return nil
	}
	err := c.sm.Close()
	c.sm = nil
	c.transport = nil
	return err
}

// Query sends a prompt and returns a range iterator over the response messages.
// Connect must be called before Query.
func (c *Client) Query(ctx context.Context, prompt string) iter.Seq2[messages.Message, error] {
	return func(yield func(messages.Message, error) bool) {
		c.mu.Lock()
		sm := c.sm
		c.mu.Unlock()
		if sm == nil {
			yield(messages.Message{}, fmt.Errorf("not connected: call Connect() first"))
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

// Interrupt sends an interrupt request to the running session.
func (c *Client) Interrupt(ctx context.Context) error {
	c.mu.Lock()
	sm := c.sm
	c.mu.Unlock()
	if sm == nil {
		return fmt.Errorf("not connected")
	}
	return sm.Interrupt(ctx)
}

// SetPermissionMode changes the permission mode for the current session.
func (c *Client) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	c.mu.Lock()
	sm := c.sm
	c.mu.Unlock()
	if sm == nil {
		return fmt.Errorf("not connected")
	}
	return sm.SetPermissionMode(ctx, mode)
}

// SetModel changes the model for the current session.
func (c *Client) SetModel(ctx context.Context, model string) error {
	c.mu.Lock()
	sm := c.sm
	c.mu.Unlock()
	if sm == nil {
		return fmt.Errorf("not connected")
	}
	return sm.SetModel(ctx, model)
}
