package sessions

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemorySessionStore is a SessionStore (with all three optional interfaces) that keeps
// transcripts in memory. It is for tests and as a reference for adapter authors; nothing
// survives the process.
type MemorySessionStore struct {
	mu    sync.Mutex
	items map[SessionKey]*memoryTranscript
}

type memoryTranscript struct {
	entries []SessionStoreEntry
	mtimeMs int64
}

// NewMemorySessionStore returns an empty store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{items: map[SessionKey]*memoryTranscript{}}
}

// Append implements SessionStore.
func (m *MemorySessionStore) Append(_ context.Context, key SessionKey, entries []SessionStoreEntry) error {
	if len(entries) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.items[key]
	if t == nil {
		t = &memoryTranscript{}
		m.items[key] = t
	}
	t.entries = append(t.entries, entries...)
	t.mtimeMs = time.Now().UnixMilli()
	return nil
}

// Load implements SessionStore.
func (m *MemorySessionStore) Load(_ context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.items[key]
	if t == nil {
		return nil, nil
	}
	return append([]SessionStoreEntry(nil), t.entries...), nil
}

// ListSessions implements SessionLister.
func (m *MemorySessionStore) ListSessions(_ context.Context, projectKey string) ([]SessionListEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []SessionListEntry{}
	for k, t := range m.items {
		if k.ProjectKey == projectKey && k.Subpath == "" {
			out = append(out, SessionListEntry{SessionID: k.SessionID, MtimeMs: t.mtimeMs})
		}
	}
	return out, nil
}

// Delete implements SessionDeleter.
func (m *MemorySessionStore) Delete(_ context.Context, key SessionKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key.Subpath != "" {
		delete(m.items, key)
		return nil
	}
	for k := range m.items {
		if k.ProjectKey == key.ProjectKey && k.SessionID == key.SessionID {
			delete(m.items, k)
		}
	}
	return nil
}

// ListSubkeys implements SubkeyLister.
func (m *MemorySessionStore) ListSubkeys(_ context.Context, key SessionKey) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []string{}
	for k := range m.items {
		if k.ProjectKey == key.ProjectKey && k.SessionID == key.SessionID && k.Subpath != "" {
			out = append(out, k.Subpath)
		}
	}
	sort.Strings(out)
	return out, nil
}
