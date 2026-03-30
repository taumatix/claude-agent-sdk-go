// Package sessions provides types and operations for managing Claude session history.
package sessions

// SessionInfo contains metadata about a stored session.
type SessionInfo struct {
	// SessionID is the unique identifier (UUID) for the session.
	SessionID string `json:"session_id"`

	// Summary is a human-readable title for the session.
	Summary string `json:"summary"`

	// LastModified is the last modification time in milliseconds since epoch.
	LastModified int64 `json:"last_modified"`

	// FileSize is the session file size in bytes (may be 0 for remote stores).
	FileSize int64 `json:"file_size,omitempty"`

	// CustomTitle is a user-set or AI-generated title.
	CustomTitle string `json:"custom_title,omitempty"`

	// FirstPrompt is the first meaningful user prompt in the session.
	FirstPrompt string `json:"first_prompt,omitempty"`

	// GitBranch is the git branch at session end.
	GitBranch string `json:"git_branch,omitempty"`

	// Cwd is the working directory for the session.
	Cwd string `json:"cwd,omitempty"`

	// Tag is a user-set session tag.
	Tag string `json:"tag,omitempty"`

	// CreatedAt is the creation time in milliseconds since epoch.
	CreatedAt int64 `json:"created_at,omitempty"`
}

// SessionMessage is a single user or assistant message from a session transcript.
type SessionMessage struct {
	// Type is "user" or "assistant".
	Type string `json:"type"`

	// UUID is the unique message identifier.
	UUID string `json:"uuid"`

	// SessionID is the session this message belongs to.
	SessionID string `json:"session_id"`

	// Message is the raw Anthropic API message payload.
	Message interface{} `json:"message"`

	// ParentToolUseID is nil for top-level messages.
	ParentToolUseID interface{} `json:"parent_tool_use_id,omitempty"`
}
