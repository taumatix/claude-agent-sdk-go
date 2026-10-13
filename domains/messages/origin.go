package messages

import (
	"encoding/json"
	"fmt"
)

// OriginKind is the "kind" of a MessageOrigin. The CLI adds kinds over time; a kind not
// listed here is still returned, and IsHuman is false for it.
type OriginKind string

const (
	OriginHuman            OriginKind = "human"
	OriginChannel          OriginKind = "channel"
	OriginPeer             OriginKind = "peer"
	OriginTaskNotification OriginKind = "task-notification"
	OriginCoordinator      OriginKind = "coordinator"
	OriginUnclassified     OriginKind = "unclassified"
	OriginObserver         OriginKind = "observer"
	OriginAutoContinuation OriginKind = "auto-continuation"
	OriginObserverActivity OriginKind = "observer-activity"
)

// TaskNotificationSubkind is the "subkind" of a task-notification origin.
type TaskNotificationSubkind string

const (
	// SubkindScheduledTrigger marks the fired prompt of a scheduled task.
	SubkindScheduledTrigger TaskNotificationSubkind = "scheduled-trigger"
	// SubkindPeerSendMessage marks a message sent from another of the user's sessions.
	SubkindPeerSendMessage TaskNotificationSubkind = "peer-send-message"
)

// MessageOrigin is the provenance of a user-role message, and on a ResultMessage of the
// message that triggered the turn. In streaming-input mode one connection interleaves the
// turns an application sends with turns the session injects on its own; Kind tells them
// apart. Only Kind is always set; the other fields depend on it, as noted on each.
type MessageOrigin struct {
	Kind OriginKind `json:"kind"`
	// Subkind is set for OriginTaskNotification when the delivery is a scheduled task's
	// prompt or a message from another session. Empty for an ordinary background task.
	Subkind TaskNotificationSubkind `json:"subkind,omitempty"`
	// Server names the MCP server a OriginChannel message arrived on.
	Server string `json:"server,omitempty"`
	// From is the sender address of an OriginPeer or OriginObserver message. It is
	// asserted by the sender: use it for routing and display, never as proof of identity.
	From string `json:"from,omitempty"`
	// Name is the sender's display name for OriginPeer, already normalised by the CLI.
	Name string `json:"name,omitempty"`
	// FromSession is the sender's host-openable session id for OriginPeer, when its host
	// provided one. A navigation target only.
	FromSession string `json:"fromSession,omitempty"`
	// SenderTaskID is the task id of the in-process background subagent that sent an
	// OriginPeer or OriginObserver message. Empty for a cross-session peer.
	SenderTaskID string `json:"senderTaskId,omitempty"`
	// Body is the decoded message body of an OriginPeer message with the envelope stripped.
	Body string `json:"body,omitempty"`
	// VerifiedPeerPID is the kernel-verified pid of the process that connected to the
	// session's local messaging socket for an OriginPeer message (for relayed traffic, the
	// relay). Nil when unverifiable.
	VerifiedPeerPID *int `json:"verifiedPeerPid,omitempty"`
}

// IsHuman reports whether the origin is a turn an application submitted. Anything the
// SDK does not recognise is not human.
func (o *MessageOrigin) IsHuman() bool { return o != nil && o.Kind == OriginHuman }

// ParseOrigin decodes the raw origin object a UserMessage or ResultMessage carries. A nil,
// empty or null input returns nil and no error: the CLI did not attribute the message.
func ParseOrigin(raw json.RawMessage) (*MessageOrigin, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var o MessageOrigin
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("decode message origin: %w", err)
	}
	return &o, nil
}

// ParsedOrigin decodes Origin. See ParseOrigin.
func (m UserMessage) ParsedOrigin() (*MessageOrigin, error) { return ParseOrigin(m.Origin) }

// ParsedOrigin decodes Origin. See ParseOrigin.
func (m ResultMessage) ParsedOrigin() (*MessageOrigin, error) { return ParseOrigin(m.Origin) }
