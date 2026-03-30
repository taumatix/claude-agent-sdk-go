package agent

import (
	"context"
	"encoding/json"
	"time"
)

// ToolPermissionHandler is called by the session manager when the CLI requests permission
// to use a tool. Return allow=true to permit the tool use, or false with a reason to deny.
type ToolPermissionHandler func(ctx context.Context, toolName string, input json.RawMessage, toolUseID string) (allow bool, reason string)

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
