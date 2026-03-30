package sessions_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/sessions"
)

// FakeStore is a test double for sessions.Store.
type FakeStore struct {
	ListSessionsFunc     func(string) ([]sessions.SessionInfo, error)
	ReadSessionFunc      func(string) ([]sessions.SessionMessage, error)
	WriteSessionMetaFunc func(string, sessions.SessionInfo) error
	DeleteSessionFunc    func(string) error
	ForkSessionFunc      func(string, int) (string, error)

	ListCalls   int
	ReadCalls   int
	WriteCalls  int
	DeleteCalls int
	ForkCalls   int
}

var _ sessions.Store = (*FakeStore)(nil)

func (f *FakeStore) ListSessions(path string) ([]sessions.SessionInfo, error) {
	f.ListCalls++
	if f.ListSessionsFunc != nil {
		return f.ListSessionsFunc(path)
	}
	return nil, nil
}

func (f *FakeStore) ReadSession(id string) ([]sessions.SessionMessage, error) {
	f.ReadCalls++
	if f.ReadSessionFunc != nil {
		return f.ReadSessionFunc(id)
	}
	return nil, nil
}

func (f *FakeStore) WriteSessionMeta(id string, info sessions.SessionInfo) error {
	f.WriteCalls++
	if f.WriteSessionMetaFunc != nil {
		return f.WriteSessionMetaFunc(id, info)
	}
	return nil
}

func (f *FakeStore) DeleteSession(id string) error {
	f.DeleteCalls++
	if f.DeleteSessionFunc != nil {
		return f.DeleteSessionFunc(id)
	}
	return nil
}

func (f *FakeStore) ForkSession(id string, upTo int) (string, error) {
	f.ForkCalls++
	if f.ForkSessionFunc != nil {
		return f.ForkSessionFunc(id, upTo)
	}
	return "new_session_id", nil
}

func makeSessions() []sessions.SessionInfo {
	return []sessions.SessionInfo{
		{SessionID: "sess_1", Summary: "First session", LastModified: 1000},
		{SessionID: "sess_2", Summary: "Second session", LastModified: 2000},
	}
}

func TestListSessions(t *testing.T) {
	store := &FakeStore{
		ListSessionsFunc: func(path string) ([]sessions.SessionInfo, error) {
			assert.Equal(t, "/my/project", path)
			return makeSessions(), nil
		},
	}

	result, err := sessions.ListSessions(store, "/my/project")
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 1, store.ListCalls)
}

func TestListSessions_Error(t *testing.T) {
	store := &FakeStore{
		ListSessionsFunc: func(string) ([]sessions.SessionInfo, error) {
			return nil, fmt.Errorf("storage error")
		},
	}

	_, err := sessions.ListSessions(store, "")
	assert.Error(t, err)
}

func TestGetSession(t *testing.T) {
	msgs := []sessions.SessionMessage{
		{Type: "user", UUID: "u1", SessionID: "sess_1"},
		{Type: "assistant", UUID: "u2", SessionID: "sess_1"},
	}
	store := &FakeStore{
		ListSessionsFunc: func(string) ([]sessions.SessionInfo, error) {
			return makeSessions(), nil
		},
		ReadSessionFunc: func(id string) ([]sessions.SessionMessage, error) {
			assert.Equal(t, "sess_1", id)
			return msgs, nil
		},
	}

	info, messages, err := sessions.GetSession(store, "sess_1")
	require.NoError(t, err)
	assert.Equal(t, "sess_1", info.SessionID)
	assert.Len(t, messages, 2)
	assert.Equal(t, 1, store.ListCalls)
	assert.Equal(t, 1, store.ReadCalls)
}

func TestGetSession_NotFound(t *testing.T) {
	store := &FakeStore{
		ListSessionsFunc: func(string) ([]sessions.SessionInfo, error) {
			return makeSessions(), nil
		},
	}

	_, _, err := sessions.GetSession(store, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRenameSession(t *testing.T) {
	var writtenID string
	var writtenInfo sessions.SessionInfo

	store := &FakeStore{
		ListSessionsFunc: func(string) ([]sessions.SessionInfo, error) {
			return makeSessions(), nil
		},
		WriteSessionMetaFunc: func(id string, info sessions.SessionInfo) error {
			writtenID = id
			writtenInfo = info
			return nil
		},
	}

	err := sessions.RenameSession(store, "sess_1", "My New Title")
	require.NoError(t, err)
	assert.Equal(t, "sess_1", writtenID)
	assert.Equal(t, "My New Title", writtenInfo.CustomTitle)
	assert.Equal(t, "My New Title", writtenInfo.Summary)
	assert.Equal(t, 1, store.WriteCalls)
}

func TestTagSession(t *testing.T) {
	var writtenInfo sessions.SessionInfo

	store := &FakeStore{
		ListSessionsFunc: func(string) ([]sessions.SessionInfo, error) {
			return makeSessions(), nil
		},
		WriteSessionMetaFunc: func(id string, info sessions.SessionInfo) error {
			writtenInfo = info
			return nil
		},
	}

	err := sessions.TagSession(store, "sess_2", "important")
	require.NoError(t, err)
	assert.Equal(t, "important", writtenInfo.Tag)
}

func TestDeleteSession(t *testing.T) {
	var deletedID string

	store := &FakeStore{
		DeleteSessionFunc: func(id string) error {
			deletedID = id
			return nil
		},
	}

	err := sessions.DeleteSession(store, "sess_1")
	require.NoError(t, err)
	assert.Equal(t, "sess_1", deletedID)
	assert.Equal(t, 1, store.DeleteCalls)
}

func TestForkSession(t *testing.T) {
	var forkedID string
	var forkedUpTo int

	store := &FakeStore{
		ForkSessionFunc: func(id string, upTo int) (string, error) {
			forkedID = id
			forkedUpTo = upTo
			return "forked_session_id", nil
		},
	}

	newID, err := sessions.ForkSession(store, "sess_1", 5)
	require.NoError(t, err)
	assert.Equal(t, "forked_session_id", newID)
	assert.Equal(t, "sess_1", forkedID)
	assert.Equal(t, 5, forkedUpTo)
	assert.Equal(t, 1, store.ForkCalls)
}

func TestForkSession_Error(t *testing.T) {
	store := &FakeStore{
		ForkSessionFunc: func(id string, upTo int) (string, error) {
			return "", fmt.Errorf("fork failed")
		},
	}

	_, err := sessions.ForkSession(store, "sess_1", 0)
	assert.Error(t, err)
}
