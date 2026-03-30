// Package jsonlines provides utilities for reading and writing newline-delimited JSON.
package jsonlines

import (
	"bufio"
	"io"
)

const maxTokenSize = 10 * 1024 * 1024 // 10MB

// Reader reads newline-delimited JSON lines from an io.Reader.
type Reader struct {
	scanner *bufio.Scanner
}

// NewReader creates a new Reader wrapping the given io.Reader.
func NewReader(r io.Reader) *Reader {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, maxTokenSize)
	scanner.Buffer(buf, maxTokenSize)
	scanner.Split(bufio.ScanLines)
	return &Reader{scanner: scanner}
}

// ReadLine returns the next line of raw bytes, or io.EOF when done.
func (r *Reader) ReadLine() ([]byte, error) {
	if r.scanner.Scan() {
		line := r.scanner.Bytes()
		// Return a copy to avoid slice reuse issues
		cp := make([]byte, len(line))
		copy(cp, line)
		return cp, nil
	}
	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}
