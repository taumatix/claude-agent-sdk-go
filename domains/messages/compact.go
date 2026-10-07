package messages

import "time"

// CompactBoundaryMessage marks the point where the CLI compacted the
// conversation, so a caller can tell why the context shrank. The CLI's other
// compact fields are internal and stay in System.Raw.
type CompactBoundaryMessage struct {
	// Trigger is what started the compaction: "manual" for /compact, the only
	// value seen live. Empty from a CLI that does not say.
	Trigger string
	// PreTokens and PostTokens are the context size before and after.
	PreTokens  int
	PostTokens int
	// Duration is how long the compaction took.
	Duration  time.Duration
	UUID      string
	SessionID string
}
