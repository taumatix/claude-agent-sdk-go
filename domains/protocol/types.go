// Package protocol defines the wire JSON types for the Claude Code CLI control protocol.
package protocol

import "encoding/json"

// MessageType identifies the type of a wire protocol message.
type MessageType string

const (
	TypeUser                 MessageType = "user"
	TypeAssistant            MessageType = "assistant"
	TypeSystem               MessageType = "system"
	TypeResult               MessageType = "result"
	TypeStreamEvent          MessageType = "stream_event"
	TypeRateLimitEvent       MessageType = "rate_limit_event"
	TypeConversationReset    MessageType = "conversation_reset"
	TypeControlRequest       MessageType = "control_request"
	TypeControlResponse      MessageType = "control_response"
	TypeControlCancelRequest MessageType = "control_cancel_request"
	TypeEnd                  MessageType = "end"
	TypeError                MessageType = "error"
)

// Envelope is a minimal struct used to sniff the type field before full parsing.
type Envelope struct {
	Type MessageType `json:"type"`
}

// RoleMessageBody is the inner message body for user/assistant messages.
type RoleMessageBody struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []ContentBlock
}

// InboundRoleMessage is used for both user and assistant messages arriving from the CLI.
type InboundRoleMessage struct {
	Type            MessageType     `json:"type"`
	SessionID       string          `json:"session_id"`
	Message         RoleMessageBody `json:"message"`
	UUID            *string         `json:"uuid,omitempty"`
	ParentToolUseID *string         `json:"parent_tool_use_id,omitempty"`
	Model           string          `json:"model,omitempty"`
	Usage           json.RawMessage `json:"usage,omitempty"`
	StopReason      *string         `json:"stop_reason,omitempty"`
	MessageID       *string         `json:"message_id,omitempty"`
	Error           *string         `json:"error,omitempty"`
	ToolUseResult   json.RawMessage `json:"tool_use_result,omitempty"`
	Origin          json.RawMessage `json:"origin,omitempty"`
}

// ContentBlockType identifies the type of a content block.
type ContentBlockType string

const (
	ContentTypeText          ContentBlockType = "text"
	ContentTypeThinking      ContentBlockType = "thinking"
	ContentTypeToolUse       ContentBlockType = "tool_use"
	ContentTypeToolResult    ContentBlockType = "tool_result"
	ContentTypeServerToolUse ContentBlockType = "server_tool_use"

	// ContentTypeServerToolResult is not a type the CLI puts on the wire. A
	// server-side tool result is named after the tool that produced it — see
	// the constants below — so this never matched an incoming block.
	//
	// Deprecated: kept so existing code compiles. Use IsServerToolResult.
	ContentTypeServerToolResult ContentBlockType = "server_tool_result"
)

// Server-side tool result block types. The API returns one block per server
// tool, each named after the tool that ran, so there is no umbrella type to
// match on. This list is the content-block switch in the `claude` binary
// (2.1.220), which enumerates every model-internal block it recognises.
const (
	ContentTypeWebSearchToolResult               ContentBlockType = "web_search_tool_result"
	ContentTypeWebFetchToolResult                ContentBlockType = "web_fetch_tool_result"
	ContentTypeAdvisorToolResult                 ContentBlockType = "advisor_tool_result"
	ContentTypeCodeExecutionToolResult           ContentBlockType = "code_execution_tool_result"
	ContentTypeBashCodeExecutionToolResult       ContentBlockType = "bash_code_execution_tool_result"
	ContentTypeTextEditorCodeExecutionToolResult ContentBlockType = "text_editor_code_execution_tool_result"
	ContentTypeToolSearchToolResult              ContentBlockType = "tool_search_tool_result"
)

var serverToolResultTypes = map[ContentBlockType]struct{}{
	ContentTypeWebSearchToolResult:               {},
	ContentTypeWebFetchToolResult:                {},
	ContentTypeAdvisorToolResult:                 {},
	ContentTypeCodeExecutionToolResult:           {},
	ContentTypeBashCodeExecutionToolResult:       {},
	ContentTypeTextEditorCodeExecutionToolResult: {},
	ContentTypeToolSearchToolResult:              {},
}

// IsServerToolResult reports whether t names a server-side tool result block.
// A server tool the CLI adds later will not be recognised here and arrives as
// an unknown block with its payload intact, rather than being discarded.
func IsServerToolResult(t ContentBlockType) bool {
	_, ok := serverToolResultTypes[t]
	return ok
}

// ContentBlock is a single block within a message's content array.
type ContentBlock struct {
	Type      ContentBlockType `json:"type"`
	Text      string           `json:"text,omitempty"`
	Thinking  string           `json:"thinking,omitempty"`
	Signature string           `json:"signature,omitempty"`
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Input     json.RawMessage  `json:"input,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	Content   json.RawMessage  `json:"content,omitempty"`
	IsError   *bool            `json:"is_error,omitempty"`
}

// SystemMessage carries metadata events from the CLI (task start, progress, etc.).
type SystemMessage struct {
	Type        MessageType     `json:"type"`
	Subtype     string          `json:"subtype"`
	Data        json.RawMessage `json:"data,omitempty"`
	TaskID      string          `json:"task_id,omitempty"`
	Description string          `json:"description,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	UUID        string          `json:"uuid,omitempty"`
}

// ResultMessage is the final message in a query session.
type ResultMessage struct {
	Type          MessageType     `json:"type"`
	Subtype       string          `json:"subtype"`
	SessionID     string          `json:"session_id"`
	DurationMS    int64           `json:"duration_ms"`
	DurationAPIMS int64           `json:"duration_api_ms"`
	IsError       bool            `json:"is_error"`
	NumTurns      int             `json:"num_turns"`
	Result        string          `json:"result,omitempty"`
	TotalCostUSD  *float64        `json:"total_cost_usd,omitempty"`
	Usage         json.RawMessage `json:"usage,omitempty"`
	StopReason    *string         `json:"stop_reason,omitempty"`
	UUID          *string         `json:"uuid,omitempty"`

	// TerminalReason says why the query loop ended ("completed", "max_turns",
	// "aborted_streaming", "aborted_tools", ...). Absent on CLI versions that
	// predate it, and on results that bypass the query loop.
	TerminalReason *string `json:"terminal_reason,omitempty"`
	// APIErrorStatus is the HTTP status of the failing API call when IsError is
	// true and Subtype is "success". Safe to log — carries no message content.
	APIErrorStatus    *int            `json:"api_error_status,omitempty"`
	StructuredOutput  json.RawMessage `json:"structured_output,omitempty"`
	ModelUsage        json.RawMessage `json:"model_usage,omitempty"`
	PermissionDenials json.RawMessage `json:"permission_denials,omitempty"`
	Errors            []string        `json:"errors,omitempty"`
	Origin            json.RawMessage `json:"origin,omitempty"`
}

// ConversationResetMessage is emitted when the session's conversation is
// replaced without ending the connection — after /clear, for instance. It
// zeroes the running totals reported on subsequent ResultMessages, so a caller
// accumulating TotalCostUSD across a long-lived session must snapshot on it.
type ConversationResetMessage struct {
	Type MessageType `json:"type"`
	// NewConversationID keys the fresh conversation. It is not the session_id
	// of subsequent messages — read that from the next message.
	NewConversationID string `json:"new_conversation_id"`
	UUID              string `json:"uuid"`
	// SessionID is the outgoing session that was reset.
	SessionID string `json:"session_id"`
}

// StreamEvent carries a partial/streaming API event.
type StreamEvent struct {
	Type            MessageType     `json:"type"`
	UUID            string          `json:"uuid"`
	SessionID       string          `json:"session_id"`
	Event           json.RawMessage `json:"event"`
	ParentToolUseID *string         `json:"parent_tool_use_id,omitempty"`
}

// RateLimitInfo contains rate-limit state details.
type RateLimitInfo struct {
	Status                string   `json:"status"`
	ResetsAt              *int64   `json:"resetsAt,omitempty"`
	RateLimitType         *string  `json:"rateLimitType,omitempty"`
	Utilization           *float64 `json:"utilization,omitempty"`
	OverageStatus         *string  `json:"overageStatus,omitempty"`
	OverageResetsAt       *int64   `json:"overageResetsAt,omitempty"`
	OverageDisabledReason *string  `json:"overageDisabledReason,omitempty"`
}

// RateLimitEvent is emitted when the rate-limit state changes.
type RateLimitEvent struct {
	Type          MessageType   `json:"type"`
	UUID          string        `json:"uuid"`
	SessionID     string        `json:"session_id"`
	RateLimitInfo RateLimitInfo `json:"rate_limit_info"`
}

// EndMessage signals the end of a session stream.
type EndMessage struct {
	Type MessageType `json:"type"`
}

// ErrorMessage carries a fatal error from the CLI.
type ErrorMessage struct {
	Type    MessageType `json:"type"`
	Message string      `json:"message"`
}

// ControlRequestEnvelope wraps an outbound control request from SDK → CLI.
type ControlRequestEnvelope struct {
	Type      MessageType     `json:"type"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
}

// ControlResponseEnvelope wraps a control response from CLI → SDK.
type ControlResponseEnvelope struct {
	Type     MessageType         `json:"type"`
	Response ControlResponseBody `json:"response"`
}

// ControlResponseBody is the inner body of a control response.
type ControlResponseBody struct {
	Subtype   string          `json:"subtype"`
	RequestID string          `json:"request_id"`
	Response  json.RawMessage `json:"response,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// ControlCancelRequest is sent to cancel a pending control request.
type ControlCancelRequest struct {
	Type      MessageType `json:"type"`
	RequestID string      `json:"request_id"`
}

// ControlRequestSubtype identifies the operation within a control request.
type ControlRequestSubtype string

const (
	SubtypeInitialize   ControlRequestSubtype = "initialize"
	SubtypeCanUseTool   ControlRequestSubtype = "can_use_tool"
	SubtypeHookCallback ControlRequestSubtype = "hook_callback"
	SubtypeInterrupt    ControlRequestSubtype = "interrupt"
	SubtypeSetPermMode  ControlRequestSubtype = "set_permission_mode"
	SubtypeSetModel     ControlRequestSubtype = "set_model"
)

// ControlRequestBase provides the subtype field common to all control request bodies.
type ControlRequestBase struct {
	Subtype ControlRequestSubtype `json:"subtype"`
}

// CanUseToolRequest is sent by the CLI to ask permission for a tool use.
type CanUseToolRequest struct {
	Subtype               ControlRequestSubtype `json:"subtype"`
	ToolName              string                `json:"tool_name"`
	Input                 json.RawMessage       `json:"input"`
	ToolUseID             string                `json:"tool_use_id"`
	PermissionSuggestions json.RawMessage       `json:"permission_suggestions,omitempty"`
}

// HookCallbackRequest is sent by the CLI to invoke a registered hook.
type HookCallbackRequest struct {
	Subtype    ControlRequestSubtype `json:"subtype"`
	CallbackID string                `json:"callback_id"`
	Input      json.RawMessage       `json:"input"`
	ToolUseID  *string               `json:"tool_use_id,omitempty"`
}

// InitializeRequestBody is the body of the initialize control request.
type InitializeRequestBody struct {
	Subtype ControlRequestSubtype  `json:"subtype"`
	Hooks   map[string]interface{} `json:"hooks"`
	Agents  map[string]interface{} `json:"agents,omitempty"`
}

// InterruptRequestBody requests an interrupt of the current operation.
type InterruptRequestBody struct {
	Subtype ControlRequestSubtype `json:"subtype"`
}

// SetPermModeRequestBody requests a change to the permission mode.
type SetPermModeRequestBody struct {
	Subtype ControlRequestSubtype `json:"subtype"`
	Mode    string                `json:"mode"`
}

// SetModelRequestBody requests a change to the model.
type SetModelRequestBody struct {
	Subtype ControlRequestSubtype `json:"subtype"`
	Model   string                `json:"model"`
}

// CanUseToolResponseBody is the SDK's response to a can_use_tool request.
type CanUseToolResponseBody struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}

// HookCallbackResponseBody is the SDK's response to a hook_callback request.
type HookCallbackResponseBody struct {
	Output string `json:"output,omitempty"`
}
