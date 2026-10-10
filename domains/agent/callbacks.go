package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

// ToolPermissionHandler is called by the session manager when the CLI requests permission
// to use a tool. Return allow=true to permit the tool use, or false with a reason to deny.
type ToolPermissionHandler func(ctx context.Context, toolName string, input json.RawMessage, toolUseID string) (allow bool, reason string)

// PermissionUpdate and PermissionRuleValue are the control protocol's permission
// changes; see protocol.PermissionUpdate for the accepted Type and Destination values.
type (
	PermissionUpdate    = protocol.PermissionUpdate
	PermissionRuleValue = protocol.PermissionRuleValue
)

// ToolPermissionRequest is what the CLI asks when it wants to use a tool. Every
// field beyond the first three is optional and empty when the CLI did not send it.
type ToolPermissionRequest struct {
	ToolName  string
	Input     json.RawMessage
	ToolUseID string
	// Suggestions are the permission changes the CLI proposes for "always allow".
	// Returning them in PermissionResult.UpdatedPermissions accepts them.
	Suggestions []PermissionUpdate
	// AgentID is set when the request comes from a subagent.
	AgentID string
	// BlockedPath is the path that triggered the request, e.g. one outside the
	// allowed directories.
	BlockedPath string
	// DecisionReason says why the request was raised, for instance the reason a
	// PreToolUse hook gave when it answered "ask".
	DecisionReason string
	// Title is the full prompt sentence ("Claude wants to read foo.txt"),
	// DisplayName a short label for the action, Description a subtitle.
	Title       string
	DisplayName string
	Description string
}

// PermissionResult answers a ToolPermissionRequest.
type PermissionResult struct {
	// Allow permits the tool use. When false, Message is shown to the model.
	Allow   bool
	Message string
	// UpdatedInput replaces the tool's input when Allow is true. Empty keeps the
	// original input.
	UpdatedInput json.RawMessage
	// UpdatedPermissions are applied by the CLI when Allow is true.
	UpdatedPermissions []PermissionUpdate
	// Interrupt stops the whole turn rather than only refusing this tool use.
	Interrupt bool
}

// ToolPermissionFunc is the richer form of ToolPermissionHandler. When
// Options.ToolPermissionFunc is set it is used and ToolPermissionHandler is not.
type ToolPermissionFunc func(ctx context.Context, req ToolPermissionRequest) PermissionResult

// HookMatcher associates a hook event matcher pattern with a handler function.
type HookMatcher struct {
	// Matcher is a tool name pattern (e.g., "Bash" or "Write|Edit"). Empty means match all.
	Matcher string

	// Handler is the function called when the hook fires.
	Handler HookHandler

	// Timeout is the maximum time allowed for the hook to respond. Zero means use default.
	Timeout time.Duration
}

// HookHandler is called when the CLI fires a registered hook event.
// It receives the callback ID and raw input payload.
// The returned map is serialized and sent back to the CLI as the hook response.
type HookHandler func(ctx context.Context, callbackID string, input json.RawMessage) (map[string]interface{}, error)
