package sessions

// Store defines the interface for session persistence backends.
type Store interface {
	// ListSessions returns all sessions stored under the given project path.
	// An empty projectPath may return all sessions.
	ListSessions(projectPath string) ([]SessionInfo, error)

	// ReadSession returns the messages for a specific session by ID.
	ReadSession(sessionID string) ([]SessionMessage, error)

	// WriteSessionMeta persists updated session metadata.
	WriteSessionMeta(sessionID string, info SessionInfo) error

	// DeleteSession removes a session and all its data.
	DeleteSession(sessionID string) error

	// ForkSession creates a new session based on an existing one up to a given
	// message index. Returns the new session ID.
	ForkSession(sessionID string, upToMessage int) (string, error)
}
