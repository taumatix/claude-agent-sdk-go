package sessions

import "context"

// SessionKey addresses one transcript in a SessionStore: a session's main transcript
// (Subpath empty) or one of its sub-transcripts, such as a subagent's
// ("subagents/agent-1").
type SessionKey struct {
	ProjectKey string
	SessionID  string
	Subpath    string
}

// SessionStoreEntry is one transcript line. Its shape is the CLI's on-disk format, which is
// internal, so a store keeps entries as opaque JSON objects ("type" is always present).
type SessionStoreEntry map[string]any

// SessionStore mirrors session transcripts to external storage. Only Append and Load are
// required; a store opts into the rest by also implementing SessionLister, SessionDeleter
// and SubkeyLister, the way upstream's optional methods work.
//
// The contract is pinned by sessionstoretest.Run, which an adapter should call from its
// own tests.
type SessionStore interface {
	// Append stores a batch of entries after those already stored under key, in call
	// order. An empty batch changes nothing. An entry with a "uuid" is an idempotency key.
	Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error

	// Load returns every entry stored under key, in order. A key never written returns
	// nil, nil. Entries must be deep-equal, as JSON, to what was appended; byte-equal
	// serialization is not required.
	Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error)
}

// SessionListEntry is one session in a SessionLister's answer.
type SessionListEntry struct {
	SessionID string
	// MtimeMs is the last-modified time in Unix epoch milliseconds.
	MtimeMs int64
}

// SessionLister is implemented by stores that can list a project's sessions.
type SessionLister interface {
	// ListSessions returns the project's sessions in unspecified order. Sub-transcripts
	// are not sessions and are left out; an unknown project returns an empty slice.
	ListSessions(ctx context.Context, projectKey string) ([]SessionListEntry, error)
}

// SessionDeleter is implemented by stores that can delete.
type SessionDeleter interface {
	// Delete removes the transcript under key. With an empty Subpath it also removes every
	// sub-transcript of that session and nothing of any other session or project; with a
	// Subpath it removes only that one. A key never written is not an error.
	Delete(ctx context.Context, key SessionKey) error
}

// SubkeyLister is implemented by stores that can list a session's sub-transcripts.
type SubkeyLister interface {
	// ListSubkeys returns the Subpaths stored under the session (ProjectKey and SessionID
	// of key; its Subpath is ignored), never the main transcript.
	ListSubkeys(ctx context.Context, key SessionKey) ([]string, error)
}
