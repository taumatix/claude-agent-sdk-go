package protocol_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

// ParseLine is exported, so a caller may hand it a buffer it intends to reuse —
// bufio.Scanner.Bytes() is the obvious case, and its contract says the slice is
// invalid after the next Scan. SystemMessage.Raw must therefore own its bytes.
//
// Nothing inside this SDK would catch a regression here: shared/jsonlines
// already returns a fresh allocation per line, so dropping the copy leaves the
// whole suite green while corrupting Raw for anyone parsing from a shared
// buffer.
func TestParseLineCopiesTheSystemMessageRaw(t *testing.T) {
	line := []byte(`{"type":"system","subtype":"task_updated","task_id":"t1",` +
		`"patch":{"status":"completed"},"uuid":"u","session_id":"s"}`)

	msg, err := protocol.ParseLine(line)
	require.NoError(t, err)
	require.NotNil(t, msg.System)

	before := string(msg.System.Raw)
	require.NotEmpty(t, before)

	// Reuse the caller's buffer, exactly as a Scanner would on the next line.
	for i := range line {
		line[i] = 'x'
	}

	assert.Equal(t, before, string(msg.System.Raw),
		"Raw aliases the caller's buffer; the next line read would corrupt it")
}
