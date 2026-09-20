// Package messages defines the public SDK message types, converted from wire protocol types.
package messages

import (
	"encoding/json"

	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

// Message holds exactly one non-nil message field.
type Message struct {
	User        *UserMessage
	Assistant   *AssistantMessage
	System      *SystemMessage
	Result      *ResultMessage
	StreamEvent *StreamEventMessage
	RateLimit   *RateLimitMessage
	ConvReset   *ConversationResetMessage
}

// UserMessage is a user-role message received from the CLI.
type UserMessage struct {
	SessionID       string
	UUID            *string
	Content         []ContentBlock
	ParentToolUseID *string
	// ToolUseResult is the CLI's raw structured result for a tool-result turn.
	// Nil on ordinary user turns.
	ToolUseResult json.RawMessage
	// Origin is the CLI's raw provenance object for this message, telling an
	// injected turn (task notification, channel or peer message) from a human
	// one. Nil when the CLI did not attribute it. Left raw because the set of
	// origin kinds grows with the CLI.
	Origin json.RawMessage
}

// ConversationResetMessage reports that the session's conversation was replaced
// without ending the connection — after /clear, for instance.
//
// The reset also zeroes the running totals on subsequent ResultMessages, so a
// caller accumulating ResultMessage.TotalCostUSD over a long-lived session must
// snapshot its tally when this arrives.
type ConversationResetMessage struct {
	// NewConversationID keys the fresh conversation. It is not the SessionID of
	// subsequent messages — read that from the next message.
	NewConversationID string
	UUID              string
	// SessionID is the outgoing session that was reset.
	SessionID string
}

// AssistantMessage is an assistant-role message received from the CLI.
type AssistantMessage struct {
	SessionID       string
	UUID            *string
	Model           string
	Content         []ContentBlock
	Usage           json.RawMessage
	StopReason      *string
	MessageID       *string
	Error           *string
	ParentToolUseID *string
}

// SystemMessage is a system metadata message from the CLI.
type SystemMessage struct {
	SessionID string
	Subtype   string
	Data      json.RawMessage
	TaskID    string
	UUID      string
}

// ResultMessage is the final message in a session, carrying cost/usage metadata.
type ResultMessage struct {
	SessionID    string
	UUID         *string
	Subtype      string
	IsError      bool
	NumTurns     int
	DurationMS   int64
	Result       string
	TotalCostUSD *float64
	StopReason   *string
	Usage        json.RawMessage

	// DurationAPIMS is the time spent in API calls, a subset of DurationMS.
	DurationAPIMS int64
	// TerminalReason says why the query loop ended ("completed", "max_turns",
	// "aborted_streaming", "aborted_tools", ...). "aborted_streaming" and
	// "aborted_tools" mean the turn was cancelled. Nil on CLI versions that
	// predate the field, and on results that bypass the query loop such as a
	// local slash command.
	TerminalReason *string
	// APIErrorStatus is the HTTP status (429, 500, 529, ...) of the failing API
	// call when IsError is true and Subtype is "success". Nil otherwise. Safe to
	// log — carries no message content.
	APIErrorStatus *int
	// StructuredOutput is the raw structured result, when the agent produced one.
	StructuredOutput json.RawMessage
	// ModelUsage is the raw per-model usage breakdown, keyed by model name.
	ModelUsage json.RawMessage
	// PermissionDenials is the raw list of tool calls denied during the turn.
	PermissionDenials json.RawMessage
	// Errors holds error strings the CLI attached to a failed result.
	Errors []string
	// Origin is the CLI's raw provenance object for the user message that
	// triggered this turn. See UserMessage.Origin.
	Origin json.RawMessage
}

// StreamEventMessage carries a partial streaming API event.
type StreamEventMessage struct {
	UUID            string
	SessionID       string
	Event           json.RawMessage
	ParentToolUseID *string
}

// RateLimitMessage is emitted when the rate-limit status changes.
type RateLimitMessage struct {
	UUID          string
	SessionID     string
	RateLimitInfo protocol.RateLimitInfo
}

// ContentBlock holds exactly one content block type.
type ContentBlock struct {
	Text             *TextBlock
	Thinking         *ThinkingBlock
	ToolUse          *ToolUseBlock
	ToolResult       *ToolResultBlock
	ServerToolUse    *ServerToolUseBlock
	ServerToolResult *ServerToolResultBlock
	Unknown          *UnknownBlock
}

// UnknownBlock is a content block this SDK version does not model — a block
// type the CLI added after this release, or one it emits that no SDK decodes
// (mcp_tool_use, container_upload, redacted_thinking, compaction, ...).
//
// It exists so a block the SDK cannot name is still visible: Raw is the block
// exactly as it arrived, so a caller can decode it without waiting for an SDK
// release. Earlier versions turned every such block into an empty TextBlock,
// which both lost the payload and invented text the model never wrote.
type UnknownBlock struct {
	// Type is the block's wire "type" field. Empty if the block carried none.
	Type protocol.ContentBlockType
	// Raw is the complete block as received, including fields above.
	Raw json.RawMessage
}

// ServerToolUseBlock is a tool the API executed server-side on the model's
// behalf (web_search, web_fetch, ...). It appears alongside ToolUseBlock in the
// content stream, but the caller never returns a result for it. Branch on Name
// to know which server tool ran.
type ServerToolUseBlock struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ServerToolResultBlock is the result of a server-side tool call. Content is
// the raw payload from the API, opaque at this layer — inspect its "type" field
// to decode a specific server tool's result schema.
type ServerToolResultBlock struct {
	ToolUseID string
	Content   json.RawMessage
	// IsError reflects the block's own "is_error" field, which server tool
	// results generally omit. A failed server tool reports the failure inside
	// Content instead, as a type ending in "_tool_result_error" carrying an
	// "error_code" — so do not read IsError == false as "the tool succeeded".
	IsError bool
	// Type is the wire block type: "web_search_tool_result",
	// "advisor_tool_result", and so on. There is no single "server_tool_result"
	// on the wire, so this is what says which server tool produced the block
	// and therefore how Content is shaped. Pair it with the matching
	// ServerToolUseBlock via ToolUseID.
	Type protocol.ContentBlockType
}

// TextBlock holds plain text content.
type TextBlock struct {
	Text string
}

// ThinkingBlock holds extended thinking content.
type ThinkingBlock struct {
	Thinking  string
	Signature string
}

// ToolUseBlock represents a tool invocation within an assistant message.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResultBlock represents the result of a tool invocation.
type ToolResultBlock struct {
	ToolUseID  string
	Content    []ContentBlock
	RawContent json.RawMessage
	IsError    bool
}
