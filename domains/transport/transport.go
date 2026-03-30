// Package transport defines the Transport interface used by the agent SDK.
package transport

import "context"

// Transport is the primary seam enabling all unit tests without a real subprocess.
// Exactly one goroutine calls Send at a time (per the session_manager design),
// but multiple goroutines MAY call Receive and Close concurrently.
type Transport interface {
	// Send writes data to the process stdin. Must be safe for concurrent use.
	Send(ctx context.Context, data []byte) error

	// Receive returns the next line of output. Returns io.EOF when the stream ends.
	Receive(ctx context.Context) ([]byte, error)

	// Close shuts down the transport, releasing all resources.
	Close() error
}
