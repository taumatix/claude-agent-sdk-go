// Package reqid generates unique request IDs for correlation.
package reqid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
)

var counter atomic.Int64

// New generates a unique request ID in the form "req_<N>_<8hex>".
func New() string {
	n := counter.Add(1)

	var b [4]byte
	_, _ = rand.Read(b[:])
	suffix := hex.EncodeToString(b[:])

	return fmt.Sprintf("req_%d_%s", n, suffix)
}
