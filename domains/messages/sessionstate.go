package messages

import "github.com/taumatix/claude-agent-sdk-go/domains/protocol"

// SessionState is the session's run state as reported by session_state_changed.
//
// A value this SDK does not name is passed through unchanged rather than
// dropped, so a newer CLI's state reaches the caller.
type SessionState string

const (
	// SessionStateIdle means no further turn is owed: the result has been
	// flushed and no background agent will wake the session. The CLI's own
	// schema calls this the authoritative turn-over signal.
	SessionStateIdle SessionState = "idle"
	// SessionStateRunning means a turn is in progress, or background work
	// will start one.
	SessionStateRunning SessionState = "running"
	// SessionStateRequiresAction means the session is blocked on the host,
	// for instance on a permission decision.
	SessionStateRequiresAction SessionState = "requires_action"
)

// SessionStateChangedMessage reports a change of the session's run state.
//
// The CLI sends it only when the subprocess environment has
// CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS set; opt in through Options.Env:
//
//	agent.Options{Env: map[string]string{"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS": "1"}}
//
// Without it, Message.SessionStateChanged is never set.
type SessionStateChangedMessage struct {
	SessionID string
	UUID      string
	State     SessionState
	// WaitingOnUser is set only by CLI hosts that report it; nil means the
	// frame did not say.
	WaitingOnUser *bool
}

// BackgroundTasksChangedMessage carries the full set of live background tasks
// after a change: a task starting, finishing, being killed, a foreground agent
// being backgrounded, or an entry's Ambient flag flipping.
//
// It is a level signal, unlike the edge-triggered TaskStarted and
// TaskNotification: replace your set with Tasks on every message rather than
// merging, and a missed bookend cannot leave a stale "running" indicator. The
// CLI sends nothing at startup, so reset to the empty set whenever the CLI
// process starts. Its ordering relative to the task_* bookends for the same
// transition is unspecified, so do not correlate the two streams.
//
// Only background tasks are listed. A subagent running in the foreground is
// absent until it is backgrounded; track TaskStarted for those.
type BackgroundTasksChangedMessage struct {
	SessionID string
	UUID      string
	// Tasks is never nil: an empty slice means no background work is live.
	Tasks []BackgroundTask
}

// BackgroundTask is one live background task.
type BackgroundTask struct {
	TaskID string
	// TaskType is the CLI's task kind, for instance "local_bash" or
	// "local_agent".
	TaskType    string
	Description string
	// Ambient marks a task that is not activity; exclude it from "is work
	// running" indicators.
	Ambient bool
}

func sessionStateFromWire(p protocol.SessionStateChangedPayload) *SessionStateChangedMessage {
	if p.State == nil {
		return nil
	}
	return &SessionStateChangedMessage{
		SessionID:     p.SessionID,
		UUID:          p.UUID,
		State:         SessionState(*p.State),
		WaitingOnUser: p.WaitingOnUser,
	}
}

func backgroundTasksFromWire(p protocol.BackgroundTasksChangedPayload) *BackgroundTasksChangedMessage {
	// A missing or null set is not the empty set: under replace semantics,
	// typing it as empty would tell the caller all background work stopped.
	// encoding/json leaves the pointer nil for both.
	if p.Tasks == nil {
		return nil
	}
	tasks := make([]BackgroundTask, 0, len(*p.Tasks))
	for _, t := range *p.Tasks {
		tasks = append(tasks, BackgroundTask{
			TaskID:      t.TaskID,
			TaskType:    t.TaskType,
			Description: t.Description,
			Ambient:     t.Ambient,
		})
	}
	return &BackgroundTasksChangedMessage{
		SessionID: p.SessionID,
		UUID:      p.UUID,
		Tasks:     tasks,
	}
}
