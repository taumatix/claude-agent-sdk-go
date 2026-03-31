package sessions_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/sessions"
)

// newTempStore creates a FilesystemStore rooted in a temp directory.
func newTempStore(t *testing.T) *sessions.FilesystemStore {
	t.Helper()
	dir := t.TempDir()
	store, err := sessions.NewFilesystemStore(dir)
	require.NoError(t, err)
	return store
}

// writeSessionFile writes a minimal JSONL session file to the store's base dir.
func writeSessionFile(t *testing.T, baseDir, sessionID string, msgs []sessions.SessionMessage) {
	t.Helper()
	path := filepath.Join(baseDir, sessionID+".jsonl")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range msgs {
		require.NoError(t, enc.Encode(m))
	}
}

// storeDirOf returns the base dir of a store via NewFilesystemStore roundtrip.
// Since FilesystemStore.baseDir is unexported, we use a temp dir we control.
func storeDirOf(t *testing.T) (store *sessions.FilesystemStore, baseDir string) {
	t.Helper()
	baseDir = t.TempDir()
	var err error
	store, err = sessions.NewFilesystemStore(baseDir)
	require.NoError(t, err)
	return store, baseDir
}

func TestFilesystemStore_ListSessions_Empty(t *testing.T) {
	store := newTempStore(t)
	sessions, err := store.ListSessions("")
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestFilesystemStore_ListSessions_ReturnsSessions(t *testing.T) {
	store, dir := storeDirOf(t)
	writeSessionFile(t, dir, "sess-001", nil)
	writeSessionFile(t, dir, "sess-002", nil)

	list, err := store.ListSessions("")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	ids := []string{list[0].SessionID, list[1].SessionID}
	assert.ElementsMatch(t, []string{"sess-001", "sess-002"}, ids)
}

func TestFilesystemStore_ReadSession_ReturnsMessages(t *testing.T) {
	store, dir := storeDirOf(t)
	msgs := []sessions.SessionMessage{
		{Type: "user", UUID: "u1", SessionID: "sess-abc"},
		{Type: "assistant", UUID: "a1", SessionID: "sess-abc"},
	}
	writeSessionFile(t, dir, "sess-abc", msgs)

	got, err := store.ReadSession("sess-abc")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "user", got[0].Type)
	assert.Equal(t, "assistant", got[1].Type)
}

func TestFilesystemStore_ReadSession_NotFound(t *testing.T) {
	store := newTempStore(t)
	_, err := store.ReadSession("nonexistent")
	assert.Error(t, err)
}

func TestFilesystemStore_WriteSessionMeta_RoundTrip(t *testing.T) {
	store, dir := storeDirOf(t)
	writeSessionFile(t, dir, "sess-meta", nil)

	info := sessions.SessionInfo{
		SessionID:   "sess-meta",
		CustomTitle: "My Session",
		Tag:         "important",
	}
	err := store.WriteSessionMeta("sess-meta", info)
	require.NoError(t, err)

	// Sidecar file must exist
	metaPath := filepath.Join(dir, "sess-meta.meta.json")
	_, err = os.Stat(metaPath)
	require.NoError(t, err)

	// Reading back via ListSessions should return the metadata
	list, err := store.ListSessions("")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "My Session", list[0].CustomTitle)
	assert.Equal(t, "important", list[0].Tag)
}

func TestFilesystemStore_DeleteSession(t *testing.T) {
	store, dir := storeDirOf(t)
	writeSessionFile(t, dir, "sess-del", nil)

	err := store.DeleteSession("sess-del")
	require.NoError(t, err)

	// File should be gone
	_, statErr := os.Stat(filepath.Join(dir, "sess-del.jsonl"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestFilesystemStore_DeleteSession_NotFound(t *testing.T) {
	store := newTempStore(t)
	err := store.DeleteSession("does-not-exist")
	assert.Error(t, err)
}

func TestFilesystemStore_ForkSession(t *testing.T) {
	store, dir := storeDirOf(t)
	msgs := []sessions.SessionMessage{
		{Type: "user", UUID: "u1", SessionID: "src"},
		{Type: "assistant", UUID: "a1", SessionID: "src"},
		{Type: "user", UUID: "u2", SessionID: "src"},
	}
	writeSessionFile(t, dir, "src", msgs)

	newID, err := store.ForkSession("src", 2)
	require.NoError(t, err)
	assert.NotEmpty(t, newID)
	assert.NotEqual(t, "src", newID)

	// Forked session should have exactly 2 messages
	forked, err := store.ReadSession(newID)
	require.NoError(t, err)
	assert.Len(t, forked, 2)
}

func TestFilesystemStore_ForkSession_ClampsToLength(t *testing.T) {
	store, dir := storeDirOf(t)
	msgs := []sessions.SessionMessage{
		{Type: "user", UUID: "u1", SessionID: "src2"},
	}
	writeSessionFile(t, dir, "src2", msgs)

	// upToMessage larger than actual message count
	newID, err := store.ForkSession("src2", 999)
	require.NoError(t, err)

	forked, err := store.ReadSession(newID)
	require.NoError(t, err)
	assert.Len(t, forked, 1)
}

func TestSanitizePath_ViaListSessions(t *testing.T) {
	// Verify that a project sub-directory is used when projectPath is non-empty.
	// We create the sub-dir manually, write a session there, and check it's listed.
	store, dir := storeDirOf(t)
	subDir := filepath.Join(dir, "Users-bob-myproject")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	writeSessionFile(t, subDir, "sess-sub", nil)

	// ListSessions with a path that sanitizes to "Users-bob-myproject"
	list, err := store.ListSessions("/Users/bob/myproject")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "sess-sub", list[0].SessionID)
}
