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
}

// UserMessage is a user-role message received from the CLI.
type UserMessage struct {
	SessionID       string
	UUID            *string
	Content         []ContentBlock
	ParentToolUseID *string
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
	Text       *TextBlock
	Thinking   *ThinkingBlock
	ToolUse    *ToolUseBlock
	ToolResult *ToolResultBlock
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
