package agent

import (
	"encoding/json"
)

// Typed hook outputs. A HookHandler returns map[string]interface{}; the types
// here build that map (Map) so the key names and nesting are not typed by hand.
// Names follow upstream's hook output TypedDicts (claude-agent-sdk-python
// types.py); they are checked against that source, not a live CLI. Zero-valued
// optional fields are omitted, so "false" and "empty" are not sent.

// HookSpecificOutput is the event-specific part of a SyncHookJSONOutput. Each
// type below implements it and sends its own hookEventName.
type HookSpecificOutput interface{ hookEvent() string }

// HookJSONOutput is what a hook returns: SyncHookJSONOutput or AsyncHookJSONOutput.
type HookJSONOutput interface {
	// Map converts the output to the map a HookHandler returns.
	Map() map[string]interface{}
}

// AsyncHookJSONOutput defers the hook's execution.
type AsyncHookJSONOutput struct {
	// AsyncTimeout is in milliseconds; zero means the CLI's default.
	AsyncTimeout int
}

// Map implements HookJSONOutput.
func (a AsyncHookJSONOutput) Map() map[string]interface{} {
	m := map[string]interface{}{"async": true}
	if a.AsyncTimeout != 0 {
		m["asyncTimeout"] = a.AsyncTimeout
	}
	return m
}

// SyncHookJSONOutput is a hook's synchronous answer: control fields, a decision,
// and optionally an event-specific part.
type SyncHookJSONOutput struct {
	// Continue false stops Claude after the hook; nil leaves the CLI's default (true).
	Continue *bool `json:"continue,omitempty"`
	// SuppressOutput hides stdout from the transcript.
	SuppressOutput bool `json:"suppressOutput,omitempty"`
	// StopReason is shown when Continue is false.
	StopReason string `json:"stopReason,omitempty"`
	// Decision is "block" to block; only that value is meaningful for most hooks.
	Decision string `json:"decision,omitempty"`
	// SystemMessage is a warning shown to the user.
	SystemMessage string `json:"systemMessage,omitempty"`
	// Reason is feedback for Claude about the decision.
	Reason string `json:"reason,omitempty"`
	// HookSpecificOutput carries event-specific controls.
	HookSpecificOutput HookSpecificOutput `json:"-"`
}

// Map implements HookJSONOutput.
func (o SyncHookJSONOutput) Map() map[string]interface{} {
	m := toMap(o)
	if o.HookSpecificOutput != nil {
		sm := toMap(o.HookSpecificOutput)
		sm["hookEventName"] = o.HookSpecificOutput.hookEvent()
		m["hookSpecificOutput"] = sm
	}
	return m
}

// toMap marshals v through its json tags. Every type passed here is built from
// strings, bools, ints, slices and maps the caller supplied; a value json cannot
// encode (a channel inside UpdatedToolOutput) surfaces as an empty map, so the
// CLI sees no output rather than the SDK panicking inside a hook.
func toMap(v interface{}) map[string]interface{} {
	b, err := json.Marshal(v)
	m := map[string]interface{}{}
	if err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// PreToolUseHookSpecificOutput is the event-specific output for PreToolUse hooks.
type PreToolUseHookSpecificOutput struct {
	// PermissionDecision: "allow", "deny", "ask" or "defer"
	PermissionDecision       string                 `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string                 `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             map[string]interface{} `json:"updatedInput,omitempty"`
	AdditionalContext        string                 `json:"additionalContext,omitempty"`
}

func (PreToolUseHookSpecificOutput) hookEvent() string { return "PreToolUse" }

// PostToolUseHookSpecificOutput is the event-specific output for PostToolUse hooks.
type PostToolUseHookSpecificOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
	// UpdatedToolOutput: Replaces the tool output before the model sees it; for built-in tools it must match the tool's output schema or the CLI keeps the original.
	UpdatedToolOutput interface{} `json:"updatedToolOutput,omitempty"`
	// UpdatedMCPToolOutput: Replaces the output of MCP tools only; prefer UpdatedToolOutput.
	UpdatedMCPToolOutput interface{} `json:"updatedMCPToolOutput,omitempty"`
}

func (PostToolUseHookSpecificOutput) hookEvent() string { return "PostToolUse" }

// PostToolUseFailureHookSpecificOutput is the event-specific output for PostToolUseFailure hooks.
type PostToolUseFailureHookSpecificOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

func (PostToolUseFailureHookSpecificOutput) hookEvent() string { return "PostToolUseFailure" }

// UserPromptSubmitHookSpecificOutput is the event-specific output for UserPromptSubmit hooks.
type UserPromptSubmitHookSpecificOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
	SessionTitle      string `json:"sessionTitle,omitempty"`
	// SuppressOriginalPrompt: With decision "block", omit the original prompt from the block message.
	SuppressOriginalPrompt bool `json:"suppressOriginalPrompt,omitempty"`
}

func (UserPromptSubmitHookSpecificOutput) hookEvent() string { return "UserPromptSubmit" }

// UserPromptExpansionHookSpecificOutput is the event-specific output for UserPromptExpansion hooks.
type UserPromptExpansionHookSpecificOutput struct {
	AdditionalContext      string `json:"additionalContext,omitempty"`
	SuppressOriginalPrompt bool   `json:"suppressOriginalPrompt,omitempty"`
}

func (UserPromptExpansionHookSpecificOutput) hookEvent() string { return "UserPromptExpansion" }

// SessionStartHookSpecificOutput is the event-specific output for SessionStart hooks.
type SessionStartHookSpecificOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
	// InitialUserMessage: Used as the first user message of the session.
	InitialUserMessage string `json:"initialUserMessage,omitempty"`
	// SessionTitle: Ignored when the session starts from clear or compact.
	SessionTitle string `json:"sessionTitle,omitempty"`
	// WatchPaths: Absolute paths to watch for FileChanged events.
	WatchPaths []string `json:"watchPaths,omitempty"`
	// ReloadSkills: Re-scan skill and command directories once SessionStart hooks finish.
	ReloadSkills bool `json:"reloadSkills,omitempty"`
}

func (SessionStartHookSpecificOutput) hookEvent() string { return "SessionStart" }

// NotificationHookSpecificOutput is the event-specific output for Notification hooks.
type NotificationHookSpecificOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

func (NotificationHookSpecificOutput) hookEvent() string { return "Notification" }

// SubagentStartHookSpecificOutput is the event-specific output for SubagentStart hooks.
type SubagentStartHookSpecificOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

func (SubagentStartHookSpecificOutput) hookEvent() string { return "SubagentStart" }

// PermissionRequestHookSpecificOutput is the event-specific output for PermissionRequest hooks.
type PermissionRequestHookSpecificOutput struct {
	Decision map[string]interface{} `json:"decision"`
}

func (PermissionRequestHookSpecificOutput) hookEvent() string { return "PermissionRequest" }

// PostToolBatchHookSpecificOutput is the event-specific output for PostToolBatch hooks.
type PostToolBatchHookSpecificOutput struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

func (PostToolBatchHookSpecificOutput) hookEvent() string { return "PostToolBatch" }

// StopHookSpecificOutput is the event-specific output for Stop hooks.
type StopHookSpecificOutput struct {
	// AdditionalContext: Non-error feedback for the model; the conversation continues.
	AdditionalContext string `json:"additionalContext,omitempty"`
}

func (StopHookSpecificOutput) hookEvent() string { return "Stop" }

// SubagentStopHookSpecificOutput is the event-specific output for SubagentStop hooks.
type SubagentStopHookSpecificOutput struct {
	// AdditionalContext: Non-error feedback for the subagent; it continues.
	AdditionalContext string `json:"additionalContext,omitempty"`
}

func (SubagentStopHookSpecificOutput) hookEvent() string { return "SubagentStop" }

// PermissionDeniedHookSpecificOutput is the event-specific output for PermissionDenied hooks.
type PermissionDeniedHookSpecificOutput struct {
	// Retry: Tells the model it may retry the denied call; ignored for denials without a classifier verdict.
	Retry bool `json:"retry,omitempty"`
}

func (PermissionDeniedHookSpecificOutput) hookEvent() string { return "PermissionDenied" }

// PreModelSwitchHookSpecificOutput is the event-specific output for PreModelSwitch hooks.
type PreModelSwitchHookSpecificOutput struct {
	// PermissionDecision: "allow" proceeds, "deny" cancels the switch.
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

func (PreModelSwitchHookSpecificOutput) hookEvent() string { return "PreModelSwitch" }

// PostModelSwitchHookSpecificOutput is the event-specific output for PostModelSwitch hooks.
type PostModelSwitchHookSpecificOutput struct {
	// AdditionalContext: Reaches the model with the next request the new model serves.
	AdditionalContext string `json:"additionalContext,omitempty"`
}

func (PostModelSwitchHookSpecificOutput) hookEvent() string { return "PostModelSwitch" }

// ElicitationHookSpecificOutput is the event-specific output for Elicitation hooks.
type ElicitationHookSpecificOutput struct {
	// Action: "accept", "decline" or "cancel".
	Action string `json:"action,omitempty"`
	// Content: Form field values; used only with action "accept".
	Content map[string]interface{} `json:"content,omitempty"`
}

func (ElicitationHookSpecificOutput) hookEvent() string { return "Elicitation" }

// ElicitationResultHookSpecificOutput is the event-specific output for ElicitationResult hooks.
type ElicitationResultHookSpecificOutput struct {
	// Action: "accept", "decline" or "cancel".
	Action  string                 `json:"action,omitempty"`
	Content map[string]interface{} `json:"content,omitempty"`
}

func (ElicitationResultHookSpecificOutput) hookEvent() string { return "ElicitationResult" }

// CwdChangedHookSpecificOutput is the event-specific output for CwdChanged hooks.
type CwdChangedHookSpecificOutput struct {
	// WatchPaths: Replaces the current dynamic watch list.
	WatchPaths []string `json:"watchPaths,omitempty"`
}

func (CwdChangedHookSpecificOutput) hookEvent() string { return "CwdChanged" }

// FileChangedHookSpecificOutput is the event-specific output for FileChanged hooks.
type FileChangedHookSpecificOutput struct {
	// WatchPaths: Replaces the current dynamic watch list.
	WatchPaths []string `json:"watchPaths,omitempty"`
}

func (FileChangedHookSpecificOutput) hookEvent() string { return "FileChanged" }

// WorktreeCreateHookSpecificOutput is the event-specific output for WorktreeCreate hooks.
type WorktreeCreateHookSpecificOutput struct {
	// WorktreePath: Absolute path of the created worktree directory (required).
	WorktreePath string `json:"worktreePath"`
}

func (WorktreeCreateHookSpecificOutput) hookEvent() string { return "WorktreeCreate" }

// MessageDisplayHookSpecificOutput is the event-specific output for MessageDisplay hooks.
type MessageDisplayHookSpecificOutput struct {
	// DisplayContent: Text displayed in place of the delta; display-only. Omit to show the original.
	DisplayContent string `json:"displayContent,omitempty"`
}

func (MessageDisplayHookSpecificOutput) hookEvent() string { return "MessageDisplay" }
