package messages

// ThinkingTokensMessage is progress while the model thinks: how large the
// current thinking block has grown, sent during the phase where the API
// otherwise streams nothing a caller can show. It is an estimate for a spinner
// or a progress pill, not the billed count, which ResultMessage's usage holds.
type ThinkingTokensMessage struct {
	// EstimatedTokens is the running total for the current thinking block.
	EstimatedTokens int
	// Delta is what this message added to it.
	Delta int
	// UserMessageUUID names the user message whose turn this is, when that
	// message carried a uuid. Empty otherwise, and from older CLIs.
	UserMessageUUID string
	UUID            string
	SessionID       string
}
