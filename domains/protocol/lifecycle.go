package protocol

import "encoding/json"

// SystemSubtype names a `system` message's subtype field.
//
// The CLI's subtype set grows with every release, so this is not exhaustive and
// is not meant to be: a subtype absent from this list still arrives as a generic
// SystemMessage with its payload intact. Only the subtypes this SDK decodes into
// a typed message are named here.
type SystemSubtype string

// Named SystemSubtype* rather than Subtype* to keep them apart from the
// ControlRequestSubtype constants in this package, which are a different
// protocol: SubtypeHookCallback is a control request the CLI sends to invoke a
// registered hook, while SystemSubtypeHookStarted is a notification that one
// began running.
const (
	SystemSubtypeTaskStarted      SystemSubtype = "task_started"
	SystemSubtypeTaskProgress     SystemSubtype = "task_progress"
	SystemSubtypeTaskUpdated      SystemSubtype = "task_updated"
	SystemSubtypeTaskNotification SystemSubtype = "task_notification"

	SystemSubtypeHookStarted  SystemSubtype = "hook_started"
	SystemSubtypeHookProgress SystemSubtype = "hook_progress"
	SystemSubtypeHookResponse SystemSubtype = "hook_response"

	SystemSubtypeSessionStateChanged    SystemSubtype = "session_state_changed"
	SystemSubtypeBackgroundTasksChanged SystemSubtype = "background_tasks_changed"

	SystemSubtypePermissionDenied SystemSubtype = "permission_denied"
	SystemSubtypeAPIRetry         SystemSubtype = "api_retry"
	SystemSubtypeThinkingTokens   SystemSubtype = "thinking_tokens"
)

// TaskUsage is the usage tally reported on task_progress and task_notification.
type TaskUsage struct {
	TotalTokens int   `json:"total_tokens"`
	ToolUses    int   `json:"tool_uses"`
	DurationMS  int64 `json:"duration_ms"`
}

// TaskStartedPayload is the `system`/`task_started` frame.
//
// Fields and optionality are taken from the zod schema the `claude` 2.1.267
// bundle carries for this subtype, and were seen populated in a live run on
// 2026-09-25. Everything is top-level: there is no `data` object.
type TaskStartedPayload struct {
	TaskID      string `json:"task_id"`
	ToolUseID   string `json:"tool_use_id,omitempty"`
	Description string `json:"description"`
	// SubagentType is set for Task-tool subagents ("Explore", ...).
	SubagentType string `json:"subagent_type,omitempty"`
	// IsBackgrounded is set only for local_agent and local_bash tasks, so nil
	// means "not applicable" rather than "foreground". A later move to the
	// background arrives as a task_updated patch, not a second task_started.
	IsBackgrounded *bool `json:"is_backgrounded,omitempty"`
	// SpawnDepth is 1 for a top-level spawn and N+1 when spawned from inside a
	// depth-N agent. Nil on tasks that are not spawned subagents.
	SpawnDepth *int   `json:"spawn_depth,omitempty"`
	TaskType   string `json:"task_type,omitempty"`
	// WorkflowName is meta.name from the workflow script, set only when
	// TaskType is "local_workflow".
	WorkflowName string `json:"workflow_name,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
	// SkipTranscript marks an ambient or housekeeping task a host should keep
	// out of the inline transcript. Absent means false.
	SkipTranscript bool `json:"skip_transcript,omitempty"`
	// Ambient marks a task that is not activity; hosts should exclude it from
	// activity indicators. Absent means false.
	Ambient   bool   `json:"ambient,omitempty"`
	UUID      string `json:"uuid,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// TaskProgressPayload is the `system`/`task_progress` frame.
type TaskProgressPayload struct {
	TaskID       string    `json:"task_id"`
	ToolUseID    string    `json:"tool_use_id,omitempty"`
	Description  string    `json:"description"`
	SubagentType string    `json:"subagent_type,omitempty"`
	Usage        TaskUsage `json:"usage"`
	LastToolName string    `json:"last_tool_name,omitempty"`
	// Summary is a one-line status for the task's row: a model-generated
	// progress summary for a local_agent task, or an MCP server's own status
	// message for a backgrounded mcp_task. Render it whenever it is present.
	Summary   string `json:"summary,omitempty"`
	UUID      string `json:"uuid,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// TaskUpdatedPayload is the `system`/`task_updated` frame: a wire-safe subset of
// the CLI's task state carrying only the fields that changed.
type TaskUpdatedPayload struct {
	TaskID string `json:"task_id"`
	// Patch is left raw so the whole line is not re-scanned to recover it, and
	// so a field the CLI adds to the patch survives into the public
	// TaskUpdatedMessage.RawPatch. Decode it into TaskPatch.
	Patch     json.RawMessage `json:"patch"`
	UUID      string          `json:"uuid,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
}

// TaskPatch holds the changed fields of a task's state. Every field is optional
// by construction: a patch reports only what moved.
type TaskPatch struct {
	Status      *string `json:"status,omitempty"`
	Description *string `json:"description,omitempty"`
	// EndTime is Unix epoch milliseconds, matching the CLI's Date.now().
	EndTime *int64 `json:"end_time,omitempty"`
	// TotalPausedMS is the cumulative time the task spent paused.
	TotalPausedMS *int64  `json:"total_paused_ms,omitempty"`
	Error         *string `json:"error,omitempty"`
	// IsBackgrounded reports a task moving between foreground and background
	// after it started; the initial value is on TaskStartedPayload.
	IsBackgrounded *bool `json:"is_backgrounded,omitempty"`
}

// TaskNotificationPayload is the `system`/`task_notification` frame, emitted
// when a task completes, fails or is stopped.
//
// Not every terminal task emits one: a background task may report completion
// only through a TaskUpdatedPayload whose patch status is terminal.
type TaskNotificationPayload struct {
	TaskID     string     `json:"task_id"`
	ToolUseID  string     `json:"tool_use_id,omitempty"`
	Status     string     `json:"status"`
	OutputFile string     `json:"output_file"`
	Summary    string     `json:"summary"`
	Usage      *TaskUsage `json:"usage,omitempty"`
	// ResourceLinks holds the resource_link content blocks of a backgrounded
	// MCP task's final result — the files it returned by reference. Left raw:
	// the element schema is not modelled by this SDK yet.
	ResourceLinks  json.RawMessage `json:"resource_links,omitempty"`
	SkipTranscript bool            `json:"skip_transcript,omitempty"`
	Ambient        bool            `json:"ambient,omitempty"`
	UUID           string          `json:"uuid,omitempty"`
	SessionID      string          `json:"session_id,omitempty"`
}

// HookEventPayload is a `system` frame for one of the three hook lifecycle
// phases: hook_started, hook_progress or hook_response.
//
// The CLI emits these only when hook events are enabled for the session. All
// three carry HookID, HookName and HookEvent; the output fields are populated
// from hook_progress onwards and ExitCode/Outcome only on hook_response.
type HookEventPayload struct {
	HookID string `json:"hook_id"`
	// HookName identifies the configured hook, e.g. "SessionStart:startup".
	HookName string `json:"hook_name"`
	// HookEvent names the lifecycle event, e.g. "SessionStart", "PreToolUse".
	// It is a different field from HookName and both are always present.
	HookEvent string `json:"hook_event"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Output    string `json:"output,omitempty"`
	// ExitCode is the hook process's exit status; set on hook_response only,
	// and optional even there.
	ExitCode *int `json:"exit_code,omitempty"`
	// Outcome is "success", "error" or "cancelled" on hook_response, empty on
	// the other two phases.
	Outcome   string `json:"outcome,omitempty"`
	UUID      string `json:"uuid,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// SessionStateChangedPayload is the `system`/`session_state_changed` frame.
//
// Taken from the zod schema in the `claude` 2.1.283 bundle and confirmed
// against a live run on 2026-09-27. The CLI sends it only when the subprocess
// has CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS set.
type SessionStateChangedPayload struct {
	// State is a pointer so a frame with no state is told apart from one
	// whose state is the empty string.
	State *string `json:"state"`
	// WaitingOnUser is sent only by the CLI's bridge emitter.
	WaitingOnUser *bool `json:"waiting_on_user,omitempty"`
	// SDKHostOnly marks a frame the CLI sent because the host set
	// CLAUDE_CODE_SDK_READS_SESSION_STATE rather than the caller's opt-in.
	SDKHostOnly bool   `json:"sdk_host_only,omitempty"`
	UUID        string `json:"uuid,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
}

// BackgroundTasksChangedPayload is the `system`/`background_tasks_changed`
// frame: every live background task after a membership change.
type BackgroundTasksChangedPayload struct {
	// Tasks is a pointer so a frame with no `tasks` key is told apart from
	// the empty set, which the CLI sends when the last background task ends.
	Tasks     *[]BackgroundTask `json:"tasks"`
	UUID      string            `json:"uuid,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
}

// PermissionDeniedPayload is the `system`/`permission_denied` frame: a tool
// call the CLI refused without asking (a deny rule, dontAsk mode, the
// auto-mode classifier, or an ask with nowhere to ask). Field names are from
// the 2.1.288 schema and a live frame from 2.1.283 and 2.1.288.
type PermissionDeniedPayload struct {
	ToolName           string `json:"tool_name"`
	ToolUseID          string `json:"tool_use_id"`
	AgentID            string `json:"agent_id,omitempty"`
	DecisionReasonType string `json:"decision_reason_type,omitempty"`
	DecisionReasonCode string `json:"decision_reason_code,omitempty"`
	DecisionReason     string `json:"decision_reason,omitempty"`
	Message            string `json:"message"`
	UUID               string `json:"uuid,omitempty"`
	SessionID          string `json:"session_id,omitempty"`
}

// APIRetryPayload is the `system`/`api_retry` frame: an API request failed
// with a retryable error and will be tried again after a delay. Fields from the
// 2.1.288 schema and live frames from 2.1.283 and 2.1.288.
type APIRetryPayload struct {
	Attempt      int   `json:"attempt"`
	MaxRetries   int   `json:"max_retries"`
	RetryDelayMS int64 `json:"retry_delay_ms"`
	// ErrorStatus is null for a failure with no HTTP response (a timeout, a
	// refused connection). A live probe also saw null with a 529 from a
	// server whose response the CLI did not read as an API error.
	ErrorStatus *int `json:"error_status"`
	// Error is a short kind, such as "overloaded" or "unknown". Kept raw: the
	// schema does not pin it to a string.
	Error      json.RawMessage `json:"error"`
	NoResponse *struct {
		WaitedMS    int64 `json:"waited_ms"`
		RetryWaitMS int64 `json:"retry_wait_ms"`
	} `json:"no_response,omitempty"`
	UUID      string `json:"uuid,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// ThinkingTokensPayload is the `system`/`thinking_tokens` frame: a live
// estimate of the current thinking block's size, sent while the model thinks.
// Fields from the 2.1.288 schema and a live 2.1.288 frame.
type ThinkingTokensPayload struct {
	EstimatedTokens      int    `json:"estimated_tokens"`
	EstimatedTokensDelta int    `json:"estimated_tokens_delta"`
	UserMessageUUID      string `json:"user_message_uuid,omitempty"`
	UUID                 string `json:"uuid,omitempty"`
	SessionID            string `json:"session_id,omitempty"`
}

// BackgroundTask is one entry of a background_tasks_changed set.
type BackgroundTask struct {
	TaskID      string `json:"task_id"`
	TaskType    string `json:"task_type"`
	Description string `json:"description"`
	// Ambient marks a task hosts should exclude from activity indicators.
	Ambient bool `json:"ambient,omitempty"`
}
