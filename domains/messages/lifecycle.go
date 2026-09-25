package messages

import (
	"encoding/json"

	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

// TaskStatus is the state of a task reported by a lifecycle message.
//
// Two vocabularies meet here. A task_updated patch reports the CLI's raw
// status, including "killed"; a task_notification reports the mapped form,
// where a killed task becomes "stopped". IsTerminal spans both so a caller
// tracking active tasks does not have to know the difference.
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusPaused  TaskStatus = "paused"

	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	// TaskStatusKilled is reported by a task_updated patch.
	TaskStatusKilled TaskStatus = "killed"
	// TaskStatusStopped is the form a task_notification reports for the same
	// transition TaskStatusKilled names.
	TaskStatusStopped TaskStatus = "stopped"
)

// IsTerminal reports whether the task has finished and should be cleared from
// any "active task" tracking.
//
// A status this SDK does not know is not terminal: a caller that cleared state
// on an unrecognised value would drop a task that is still running, which is
// the worse of the two mistakes.
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusKilled, TaskStatusStopped:
		return true
	default:
		return false
	}
}

// TaskStartedMessage reports that a task began.
//
// It arrives alongside a SystemMessage with subtype "task_started"; both fields
// of the enclosing Message are populated, so code written against System keeps
// working.
type TaskStartedMessage struct {
	SessionID string
	UUID      string
	TaskID    string
	// ToolUseID joins the task back to the tool call that spawned it.
	ToolUseID   string
	Description string
	// SubagentType is set for Task-tool subagents ("Explore", ...).
	SubagentType string
	// IsBackgrounded is nil when the CLI did not report it — it is set only for
	// local_agent and local_bash tasks, so nil means "not applicable" rather
	// than "foreground".
	IsBackgrounded *bool
	// SpawnDepth is 1 for a top-level spawn and N+1 when spawned from inside a
	// depth-N agent. Nil on tasks that are not spawned subagents.
	SpawnDepth *int
	TaskType   string
	// WorkflowName is set only when TaskType is "local_workflow".
	WorkflowName string
	Prompt       string
	// SkipTranscript marks a housekeeping task a host should keep out of the
	// inline transcript.
	SkipTranscript bool
	// Ambient marks a task that is not activity; hosts should exclude it from
	// activity indicators.
	Ambient bool
}

// TaskProgressMessage reports a running task's progress.
type TaskProgressMessage struct {
	SessionID    string
	UUID         string
	TaskID       string
	ToolUseID    string
	Description  string
	SubagentType string
	Usage        TaskUsage
	LastToolName string
	// Summary is a one-line status for the task's row, when the CLI supplies
	// one. Render it whenever it is non-empty, whatever the task type.
	Summary string
}

// TaskUsage is the running tally reported on task progress and notifications.
type TaskUsage struct {
	TotalTokens int
	ToolUses    int
	DurationMS  int64
}

// TaskUpdatedMessage reports a change to a background task's state.
//
// This is the message that matters most for tracking subagents: a task's
// terminal state can arrive here with no accompanying TaskNotificationMessage —
// a task stopped via TaskStop reports TaskStatusKilled in its patch and the
// matching notification is sometimes suppressed. Clear active-task state on a
// terminal status from either message.
type TaskUpdatedMessage struct {
	SessionID string
	UUID      string
	TaskID    string
	// Status is the patch's status, or "" when the patch carried none — a patch
	// reporting only end_time or a description leaves it empty rather than
	// guessing. "" is not terminal.
	Status TaskStatus
	Patch  TaskPatch
	// RawPatch is the patch object as it arrived. The CLI may add fields to it;
	// this is how to read one before this SDK models it.
	RawPatch json.RawMessage
}

// TaskPatch holds the fields of a task's state that changed. Every field is nil
// unless this patch reported it.
type TaskPatch struct {
	Status         *TaskStatus
	Description    *string
	EndTime        *int64
	TotalPausedMS  *int64
	Error          *string
	IsBackgrounded *bool
}

// TaskNotificationMessage reports that a task completed, failed or was stopped.
//
// Not every terminal task emits one — see TaskUpdatedMessage.
type TaskNotificationMessage struct {
	SessionID string
	UUID      string
	TaskID    string
	ToolUseID string
	// Status is "completed", "failed" or "stopped".
	Status     TaskStatus
	OutputFile string
	Summary    string
	// Usage is nil when the CLI did not report a tally.
	Usage *TaskUsage
	// ResourceLinks holds the resource_link content blocks of a backgrounded
	// MCP task's final result — the files it returned by reference. Nil when
	// the result had none or the task is any other type. Left raw: this SDK
	// does not model the element schema yet.
	ResourceLinks  json.RawMessage
	SkipTranscript bool
	Ambient        bool
}

// HookPhase names which of the three hook lifecycle events arrived.
type HookPhase string

const (
	// HookPhaseStarted is emitted when a hook begins executing.
	HookPhaseStarted HookPhase = "hook_started"
	// HookPhaseProgress is emitted periodically while a hook runs, carrying its
	// output so far. Upstream's Python SDK does not model this phase; the CLI
	// emits it from an interval timer, so a hook that runs for several seconds
	// produces several.
	HookPhaseProgress HookPhase = "hook_progress"
	// HookPhaseResponse is emitted when a hook completes, and is the only phase
	// that carries ExitCode and Outcome.
	HookPhaseResponse HookPhase = "hook_response"
)

// HookOutcome is how a hook finished, reported on HookPhaseResponse only.
type HookOutcome string

const (
	HookOutcomeSuccess   HookOutcome = "success"
	HookOutcomeError     HookOutcome = "error"
	HookOutcomeCancelled HookOutcome = "cancelled"
)

// HookEventMessage reports a hook lifecycle event.
//
// The CLI emits these only when hook events are enabled for the session, so a
// caller that never sees one is not necessarily missing them.
type HookEventMessage struct {
	SessionID string
	UUID      string
	Phase     HookPhase
	HookID    string
	// HookName identifies the configured hook, e.g. "SessionStart:startup".
	HookName string
	// HookEventName names the lifecycle event, e.g. "SessionStart",
	// "PreToolUse". It is a different field from HookName — a hook's name
	// commonly embeds the event but is not equal to it.
	HookEventName string
	Stdout        string
	Stderr        string
	Output        string
	// ExitCode is the hook process's exit status. Nil except on
	// HookPhaseResponse, and optional even there.
	ExitCode *int
	// Outcome is set on HookPhaseResponse only; "" on the other phases.
	Outcome HookOutcome
}

// systemPayloadFromWire decodes a system message's typed payload onto msg.
//
// A lifecycle event must never fail the stream, so a payload that will not
// decode is reported by leaving the typed field nil: the caller still receives
// the generic SystemMessage with Raw intact. An unrecognised subtype is not an
// error either — it is the forward-compatibility path.
func systemPayloadFromWire(msg *Message, m *protocol.SystemMessage) {
	raw := m.Raw
	if len(raw) == 0 {
		return
	}

	switch protocol.SystemSubtype(m.Subtype) {
	case protocol.SubtypeTaskStarted:
		var p protocol.TaskStartedPayload
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		msg.TaskStarted = &TaskStartedMessage{
			SessionID:      p.SessionID,
			UUID:           p.UUID,
			TaskID:         p.TaskID,
			ToolUseID:      p.ToolUseID,
			Description:    p.Description,
			SubagentType:   p.SubagentType,
			IsBackgrounded: p.IsBackgrounded,
			SpawnDepth:     p.SpawnDepth,
			TaskType:       p.TaskType,
			WorkflowName:   p.WorkflowName,
			Prompt:         p.Prompt,
			SkipTranscript: p.SkipTranscript,
			Ambient:        p.Ambient,
		}

	case protocol.SubtypeTaskProgress:
		var p protocol.TaskProgressPayload
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		msg.TaskProgress = &TaskProgressMessage{
			SessionID:    p.SessionID,
			UUID:         p.UUID,
			TaskID:       p.TaskID,
			ToolUseID:    p.ToolUseID,
			Description:  p.Description,
			SubagentType: p.SubagentType,
			Usage:        taskUsageFromWire(p.Usage),
			LastToolName: p.LastToolName,
			Summary:      p.Summary,
		}

	case protocol.SubtypeTaskUpdated:
		var p protocol.TaskUpdatedPayload
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		// The patch is re-extracted rather than re-marshalled, so a field the
		// CLI added and TaskPatch does not model survives on RawPatch.
		var envelope struct {
			Patch json.RawMessage `json:"patch"`
		}
		_ = json.Unmarshal(raw, &envelope)

		patch := TaskPatch{
			Description:    p.Patch.Description,
			EndTime:        p.Patch.EndTime,
			TotalPausedMS:  p.Patch.TotalPausedMS,
			Error:          p.Patch.Error,
			IsBackgrounded: p.Patch.IsBackgrounded,
		}
		var status TaskStatus
		if p.Patch.Status != nil {
			status = TaskStatus(*p.Patch.Status)
			patch.Status = &status
		}
		msg.TaskUpdated = &TaskUpdatedMessage{
			SessionID: p.SessionID,
			UUID:      p.UUID,
			TaskID:    p.TaskID,
			Status:    status,
			Patch:     patch,
			RawPatch:  envelope.Patch,
		}

	case protocol.SubtypeTaskNotification:
		var p protocol.TaskNotificationPayload
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		var usage *TaskUsage
		if p.Usage != nil {
			u := taskUsageFromWire(*p.Usage)
			usage = &u
		}
		msg.TaskNotification = &TaskNotificationMessage{
			SessionID:      p.SessionID,
			UUID:           p.UUID,
			TaskID:         p.TaskID,
			ToolUseID:      p.ToolUseID,
			Status:         TaskStatus(p.Status),
			OutputFile:     p.OutputFile,
			Summary:        p.Summary,
			Usage:          usage,
			ResourceLinks:  p.ResourceLinks,
			SkipTranscript: p.SkipTranscript,
			Ambient:        p.Ambient,
		}

	case protocol.SubtypeHookStarted, protocol.SubtypeHookProgress, protocol.SubtypeHookResponse:
		var p protocol.HookEventPayload
		if json.Unmarshal(raw, &p) != nil {
			return
		}
		msg.HookEvent = &HookEventMessage{
			SessionID:     p.SessionID,
			UUID:          p.UUID,
			Phase:         HookPhase(m.Subtype),
			HookID:        p.HookID,
			HookName:      p.HookName,
			HookEventName: p.HookEvent,
			Stdout:        p.Stdout,
			Stderr:        p.Stderr,
			Output:        p.Output,
			ExitCode:      p.ExitCode,
			Outcome:       HookOutcome(p.Outcome),
		}
	}
}

func taskUsageFromWire(u protocol.TaskUsage) TaskUsage {
	return TaskUsage{
		TotalTokens: u.TotalTokens,
		ToolUses:    u.ToolUses,
		DurationMS:  u.DurationMS,
	}
}
