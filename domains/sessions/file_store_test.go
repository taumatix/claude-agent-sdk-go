package sessions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSessionStoreSurvivesANewStoreOverTheSameDirectory(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	key := SessionKey{ProjectKey: "p", SessionID: "s1"}
	require.NoError(t, NewFileSessionStore(dir).Append(ctx, key, []SessionStoreEntry{user("first"), user("second")}))
	require.NoError(t, NewFileSessionStore(dir).Append(ctx, SessionKey{ProjectKey: "p", SessionID: "s1", Subpath: "subagents/agent-1"}, []SessionStoreEntry{user("sub")}))

	reopened := NewFileSessionStore(dir)
	got, err := reopened.Load(ctx, key)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	subs, err := reopened.ListSubkeys(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, []string{"subagents/agent-1"}, subs)

	infos, err := ListSessionsFromStore(ctx, reopened, "p", "/proj", 0, 0)
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, "first", infos[0].FirstPrompt)
}

func TestFileSessionStoreKeysCannotEscapeTheDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "store")
	s := NewFileSessionStore(dir)
	key := SessionKey{ProjectKey: "../../outside", SessionID: "../s", Subpath: "../../x"}
	require.NoError(t, s.Append(context.Background(), key, []SessionStoreEntry{user("x")}))
	var outside []string
	require.NoError(t, filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !isUnder(dir, p) {
			outside = append(outside, p)
		}
		return err
	}))
	assert.Empty(t, outside)
	got, err := s.Load(context.Background(), key)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func isUnder(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && len(rel) > 0 && rel[0] != '.'
}

// A summary older than the file system's mtime for its transcript (a crash between the
// two writes, or a clock that disagrees) must be refolded from Load; a newer one is trusted.
func TestListingOnAFileStoreFollowsTheFileSystemClock(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := NewFileSessionStore(dir)
	for _, id := range []string{"stale", "fresh"} {
		require.NoError(t, s.Append(ctx, SessionKey{ProjectKey: "p", SessionID: id}, []SessionStoreEntry{user("real " + id)}))
	}
	for _, id := range []string{"stale", "fresh"} {
		sum := filepath.Join(s.sessionDir("p", id), "summary.json")
		raw, err := os.ReadFile(sum)
		require.NoError(t, err)
		bad := []byte(`{"session_id":"` + id + `","mtime":`)
		var mtime string
		if id == "stale" {
			mtime = "1" // behind the transcript's mtime
		} else {
			mtime = "99999999999999" // ahead of it
		}
		bad = append(bad, []byte(mtime+`,"data":{"first_prompt":"FROM SUMMARY","first_prompt_locked":true}}`)...)
		require.NotEqual(t, raw, bad)
		require.NoError(t, os.WriteFile(sum, bad, 0o600))
	}
	got, err := ListSessionsFromStore(ctx, s, "p", "", 0, 0)
	require.NoError(t, err)
	by := map[string]SessionInfo{}
	for _, g := range got {
		by[g.SessionID] = g
	}
	require.Len(t, by, 2)
	assert.Equal(t, "real stale", by["stale"].Summary, "a stale summary is refolded from the transcript")
	assert.Equal(t, "FROM SUMMARY", by["fresh"].Summary, "a summary newer than the transcript is trusted")

}

func TestListingOnAFileStoreGapFillsAMissingSummary(t *testing.T) {
	ctx := context.Background()
	s := NewFileSessionStore(t.TempDir())
	require.NoError(t, s.Append(ctx, SessionKey{ProjectKey: "p", SessionID: "a"}, []SessionStoreEntry{user("hello")}))
	require.NoError(t, os.Remove(filepath.Join(s.sessionDir("p", "a"), "summary.json")))
	got, err := ListSessionsFromStore(ctx, s, "p", "", 0, 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "hello", got[0].Summary)
}
