package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Expected values are what the CLI's own JavaScript produces for the same input.
func TestSanitizePathMatchesTheCLI(t *testing.T) {
	assert.Equal(t, "-Users-a-my-repo-v2", sanitizePath("/Users/a/my_repo.v2"))
	assert.Equal(t, "-private-tmp-claude-504--Users-taumatix-bootstrap", sanitizePath("/private/tmp/claude-504/-Users/taumatix/bootstrap"))
	assert.Equal(t, "C--Users-me-proj", sanitizePath(`C:\Users\me\proj`))

	long := "/a/" + strings.Repeat("b", 250)
	got := sanitizePath(long)
	assert.Len(t, got, 207)
	assert.True(t, strings.HasSuffix(got, "bbbbbbbbbbbbbbbbbbbbbbb-hvza71"), got)
}

func TestSimpleHashMatchesTheCLI(t *testing.T) {
	assert.Equal(t, "yzc9g", simpleHash("/Users/taumatix/work/x"))
	assert.Equal(t, "1pdoem", simpleHash("héllo"))
	assert.Equal(t, "0", simpleHash(""))
}

func TestProjectKeyForDirectoryResolvesTheDirectory(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(real, 0o755))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(real, link))

	want := ProjectKeyForDirectory(real)
	assert.Equal(t, want, ProjectKeyForDirectory(link), "a symlink and its target are one project")
	assert.True(t, strings.HasPrefix(want, "-"), "an absolute path keys with a leading dash: %s", want)
	wd, _ := os.Getwd()
	assert.Equal(t, ProjectKeyForDirectory(wd), ProjectKeyForDirectory(""))
}

func writeSession(t *testing.T, dir, id string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	line := `{"type":"user","message":{"role":"user","content":"hello"},"sessionId":"` + id + `"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(line), 0o644))
}

// The CLI files a project's sessions under its sanitized path, leading dash included;
// a project-scoped listing has to look there.
func TestListSessionsFindsAProjectTheCLIFiled(t *testing.T) {
	base := t.TempDir()
	store, err := NewFilesystemStore(base)
	require.NoError(t, err)
	writeSession(t, filepath.Join(base, "-Users-me-my-repo-v2"), "11111111-1111-4111-8111-111111111111")

	got, err := store.ListSessions("/Users/me/my_repo.v2")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", got[0].SessionID)
}

func TestListSessionsStillFindsADirectoryNamedTheOldWay(t *testing.T) {
	base := t.TempDir()
	store, _ := NewFilesystemStore(base)
	writeSession(t, filepath.Join(base, "Users-me-proj"), "22222222-2222-4222-8222-222222222222")

	got, err := store.ListSessions("/Users/me/proj")
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestListSessionsFindsALongPathDespiteADifferentHash(t *testing.T) {
	base := t.TempDir()
	store, _ := NewFilesystemStore(base)
	long := "/a/" + strings.Repeat("b", 250)
	cli := sanitizePath(long)[:maxSanitizedLength] + "-differenthash"
	writeSession(t, filepath.Join(base, cli), "33333333-3333-4333-8333-333333333333")

	got, err := store.ListSessions(long)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestListSessionsOfAnUnknownProjectIsEmpty(t *testing.T) {
	base := t.TempDir()
	store, _ := NewFilesystemStore(base)
	writeSession(t, filepath.Join(base, "-Users-me-other"), "44444444-4444-4444-8444-444444444444")
	got, err := store.ListSessions("/Users/me/proj")
	require.NoError(t, err)
	assert.Empty(t, got)
}
