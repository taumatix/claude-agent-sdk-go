package jsonlines

import (
	"io"
	"sync"
)

// Writer writes newline-delimited JSON lines to an io.Writer.
// All writes are protected by a mutex to prevent interleaving.
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriter creates a new Writer wrapping the given io.Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteLine writes data followed by a newline character.
func (w *Writer) WriteLine(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	buf := make([]byte, len(data)+1)
	copy(buf, data)
	buf[len(data)] = '\n'

	_, err := w.w.Write(buf)
	return err
}
