package agent

import (
	"encoding/json"
	"fmt"
)

// The typed hook inputs below are decoded from the raw JSON a HookHandler
// receives, by ParseHookInput. Field names follow upstream's hook input
// TypedDicts (claude-agent-sdk-python types.py); they are checked against that
// source, not against a live CLI. A key the CLI sends and this SDK does not
// model stays reachable in the raw input the handler was given.

// HookEffort is the reasoning effort in effect when a hook fires.
type HookEffort struct {
	// Level is e.g. "low", "high" or "max", after any downgrade for the model.
	Level string `json:"level"`
}

// BaseHookInput holds the fields present across hook events.
type BaseHookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PermissionMode string `json:"permission_mode,omitempty"`
	// PromptID is the UUID of the user prompt being processed; absent until the
	// first user input.
	PromptID string `json:"prompt_id,omitempty"`
	// Effort is set for hooks inside a tool-use context on models that support effort.
	Effort        *HookEffort `json:"effort,omitempty"`
	HookEventName string      `json:"hook_event_name"`
}

// EventName returns the hook event this input belongs to.
func (b BaseHookInput) EventName() string { return b.HookEventName }

// HookInput is any value ParseHookInput returns. Switch on its concrete type.
type HookInput interface{ EventName() string }

// PreToolUseHookInput is the input of a PreToolUse hook.
type PreToolUseHookInput struct {
	BaseHookInput
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	ToolUseID string          `json:"tool_use_id"`
	// AgentID is set only inside a sub-agent; AgentType also on the main thread
	// of a session started with --agent.
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

// PostToolUseHookInput is the input of a PostToolUse hook.
type PostToolUseHookInput struct {
	BaseHookInput
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
	ToolUseID    string          `json:"tool_use_id"`
	// DurationMS is the tool's execution time, excluding permission prompts and hooks.
	DurationMS int    `json:"duration_ms,omitempty"`
	AgentID    string `json:"agent_id,omitempty"`
	AgentType  string `json:"agent_type,omitempty"`
}

// PostToolUseFailureHookInput is the input of a PostToolUseFailure hook.
type PostToolUseFailureHookInput struct {
	BaseHookInput
	ToolName    string          `json:"tool_name"`
	ToolInput   json.RawMessage `json:"tool_input"`
	ToolUseID   string          `json:"tool_use_id"`
	Error       string          `json:"error"`
	IsInterrupt bool            `json:"is_interrupt,omitempty"`
	DurationMS  int             `json:"duration_ms,omitempty"`
	AgentID     string          `json:"agent_id,omitempty"`
	AgentType   string          `json:"agent_type,omitempty"`
}

// UserPromptSubmitHookInput is the input of a UserPromptSubmit hook.
type UserPromptSubmitHookInput struct {
	BaseHookInput
	Prompt string `json:"prompt"`
}

// HookBackgroundTask is an in-flight background task listed on Stop and
// SubagentStop inputs. Description and Command are capped at 1000 characters; a
// clipped value ends with a "… [+N chars]" marker.
type HookBackgroundTask struct {
	ID string `json:"id"`
	// Type is a label such as "shell", "subagent", "monitor" or "workflow".
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
	Server      string `json:"server,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Name        string `json:"name,omitempty"`
}

// SessionCronSummary is a session-scoped scheduled wakeup listed on Stop and
// SubagentStop inputs.
type SessionCronSummary struct {
	ID string `json:"id"`
	// Schedule is a cron expression.
	Schedule string `json:"schedule"`
	// Recurring is false for one-shot wakeups.
	Recurring bool `json:"recurring"`
	// Prompt is capped at 1000 characters; a clipped value ends with a marker.
	Prompt string `json:"prompt"`
}

// StopHookInput is the input of a Stop hook.
type StopHookInput struct {
	BaseHookInput
	StopHookActive       bool                 `json:"stop_hook_active"`
	LastAssistantMessage string               `json:"last_assistant_message,omitempty"`
	BackgroundTasks      []HookBackgroundTask `json:"background_tasks,omitempty"`
	SessionCrons         []SessionCronSummary `json:"session_crons,omitempty"`
}

// SubagentStopHookInput is the input of a SubagentStop hook.
type SubagentStopHookInput struct {
	BaseHookInput
	StopHookActive       bool                 `json:"stop_hook_active"`
	AgentID              string               `json:"agent_id"`
	AgentTranscriptPath  string               `json:"agent_transcript_path"`
	AgentType            string               `json:"agent_type"`
	LastAssistantMessage string               `json:"last_assistant_message,omitempty"`
	BackgroundTasks      []HookBackgroundTask `json:"background_tasks,omitempty"`
	SessionCrons         []SessionCronSummary `json:"session_crons,omitempty"`
}

// PreCompactHookInput is the input of a PreCompact hook.
type PreCompactHookInput struct {
	BaseHookInput
	// Trigger is "manual" or "auto".
	Trigger string `json:"trigger"`
	// CustomInstructions is nil when none were given.
	CustomInstructions *string `json:"custom_instructions"`
}

// NotificationHookInput is the input of a Notification hook.
type NotificationHookInput struct {
	BaseHookInput
	Message          string `json:"message"`
	Title            string `json:"title,omitempty"`
	NotificationType string `json:"notification_type"`
}

// SubagentStartHookInput is the input of a SubagentStart hook.
type SubagentStartHookInput struct {
	BaseHookInput
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
}

// PermissionRequestHookInput is the input of a PermissionRequest hook.
type PermissionRequestHookInput struct {
	BaseHookInput
	ToolName    string          `json:"tool_name"`
	ToolInput   json.RawMessage `json:"tool_input"`
	AgentID     string          `json:"agent_id,omitempty"`
	AgentType   string          `json:"agent_type,omitempty"`
	Suggestions json.RawMessage `json:"permission_suggestions,omitempty"`
}

// UnknownHookInput is what ParseHookInput returns for an event this SDK has no
// type for, so a newer CLI's hook still reaches the handler.
type UnknownHookInput struct {
	BaseHookInput
	// Raw is the whole input.
	Raw json.RawMessage `json:"-"`
}

// ParseHookInput decodes the raw input a HookHandler receives into the typed
// input for its hook_event_name. An event with no type here yields
// *UnknownHookInput; malformed JSON, or an input with no hook_event_name, is an
// error.
func ParseHookInput(raw json.RawMessage) (HookInput, error) {
	var base BaseHookInput
	if err := json.Unmarshal(raw, &base); err != nil {
		return nil, fmt.Errorf("agent: hook input: %w", err)
	}
	if base.HookEventName == "" {
		return nil, fmt.Errorf("agent: hook input has no hook_event_name")
	}
	var out HookInput
	switch base.HookEventName {
	case "PreToolUse":
		out = &PreToolUseHookInput{}
	case "PostToolUse":
		out = &PostToolUseHookInput{}
	case "PostToolUseFailure":
		out = &PostToolUseFailureHookInput{}
	case "UserPromptSubmit":
		out = &UserPromptSubmitHookInput{}
	case "Stop":
		out = &StopHookInput{}
	case "SubagentStop":
		out = &SubagentStopHookInput{}
	case "PreCompact":
		out = &PreCompactHookInput{}
	case "Notification":
		out = &NotificationHookInput{}
	case "SubagentStart":
		out = &SubagentStartHookInput{}
	case "PermissionRequest":
		out = &PermissionRequestHookInput{}
	default:
		return &UnknownHookInput{BaseHookInput: base, Raw: append(json.RawMessage(nil), raw...)}, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("agent: %s hook input: %w", base.HookEventName, err)
	}
	return out, nil
}
