package sessions

import "fmt"

// ListSessions returns all sessions for the given project path.
func ListSessions(store Store, projectPath string) ([]SessionInfo, error) {
	return store.ListSessions(projectPath)
}

// GetSession returns metadata and messages for a specific session.
func GetSession(store Store, sessionID string) (*SessionInfo, []SessionMessage, error) {
	sessions, err := store.ListSessions("")
	if err != nil {
		return nil, nil, fmt.Errorf("list sessions: %w", err)
	}

	var found *SessionInfo
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		return nil, nil, fmt.Errorf("session %s not found", sessionID)
	}

	msgs, err := store.ReadSession(sessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("read session messages: %w", err)
	}

	return found, msgs, nil
}

// RenameSession updates the summary/title of a session.
func RenameSession(store Store, sessionID, newTitle string) error {
	sessions, err := store.ListSessions("")
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	var found *SessionInfo
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("session %s not found", sessionID)
	}

	found.CustomTitle = newTitle
	found.Summary = newTitle
	return store.WriteSessionMeta(sessionID, *found)
}

// TagSession sets the tag for a session.
func TagSession(store Store, sessionID, tag string) error {
	sessions, err := store.ListSessions("")
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	var found *SessionInfo
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("session %s not found", sessionID)
	}

	found.Tag = tag
	return store.WriteSessionMeta(sessionID, *found)
}

// DeleteSession removes a session.
func DeleteSession(store Store, sessionID string) error {
	return store.DeleteSession(sessionID)
}

// ForkSession creates a new session based on an existing one up to a given message index.
// Returns the new session ID.
func ForkSession(store Store, sessionID string, upToMessage int) (string, error) {
	return store.ForkSession(sessionID, upToMessage)
}
