package sessions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProjects lays out what the CLI writes: a main transcript, a subagent
// transcript with its sidecar, and a nested one without.
func writeProjects(t *testing.T) (projects, project string) {
	t.Helper()
	projects = t.TempDir()
	project = filepath.Join(projects, "-work-app")
	line := func(typ, uuid, parent string) string {
		p := ""
		if parent != "" {
			p = fmt.Sprintf(`"parentUuid":%q,`, parent)
		}
		return fmt.Sprintf(`{"type":%q,"uuid":%q,%s"sessionId":%q,"message":{"role":%q,"content":%q}}`, typ, uuid, p, mutID, typ, uuid)
	}
	write := func(rel string, lines ...string) {
		p := filepath.Join(project, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	}
	write(mutID+".jsonl", line("user", "u1", ""), "", line("assistant", "a1", "u1"), line("user", "u2", "a1"))
	write(mutID+"/subagents/agent-s1.jsonl", line("user", "s1", ""), line("assistant", "s2", "s1"))
	write(mutID+"/subagents/agent-s1.meta.json", `{"toolUseId":"toolu_9","parentAgentId":"root","type":"shadowed"}`)
	write(mutID+"/subagents/workflows/run-1/agent-w1.jsonl", line("user", "w1", ""))
	write(mutID+"/subagents/workflows/run-1/agent-w1.meta.json", `not json`)
	return projects, project
}

func TestImportSessionToStoreRoundTripsThroughRealStores(t *testing.T) {
	projects, _ := writeProjects(t)
	ctx := context.Background()
	stores := map[string]SessionStore{
		"memory": NewMemorySessionStore(),
		"file":   NewFileSessionStore(t.TempDir()),
	}
	for name, store := range stores {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, ImportSessionToStore(ctx, mutID, store, ImportOptions{ProjectsDir: projects, BatchSize: 2}))

			// Keyed under the on-disk project directory name, blank line skipped.
			main, err := store.Load(ctx, SessionKey{ProjectKey: "-work-app", SessionID: mutID})
			require.NoError(t, err)
			require.Len(t, main, 3)
			assert.Equal(t, "u1", main[0]["uuid"])

			msgs, err := GetSessionMessagesFromStore(ctx, store, "-work-app", mutID, 0, 0)
			require.NoError(t, err)
			require.Len(t, msgs, 3)

			ids, err := ListSubagentsFromStore(ctx, store, "-work-app", mutID)
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"s1", "w1"}, ids)

			sub, err := GetSubagentMessagesFromStore(ctx, store, "-work-app", mutID, "s1", 0, 0)
			require.NoError(t, err)
			require.Len(t, sub, 2)
			assert.Equal(t, "toolu_9", sub[0].ParentToolUseID, "the sidecar must come across as agent_metadata")

			raw, err := store.Load(ctx, SessionKey{ProjectKey: "-work-app", SessionID: mutID, Subpath: "subagents/agent-s1"})
			require.NoError(t, err)
			assert.Equal(t, "agent_metadata", raw[len(raw)-1]["type"], "a stray type in the sidecar must not shadow it")

			nested, err := store.Load(ctx, SessionKey{ProjectKey: "-work-app", SessionID: mutID, Subpath: "subagents/workflows/run-1/agent-w1"})
			require.NoError(t, err)
			assert.Len(t, nested, 1, "a corrupt sidecar counts as absent")
		})
	}
}

func TestImportSessionToStoreOptionsAndRefusals(t *testing.T) {
	projects, project := writeProjects(t)
	ctx := context.Background()

	m := NewMemorySessionStore()
	require.NoError(t, ImportSessionToStore(ctx, mutID, m, ImportOptions{ProjectsDir: projects, SkipSubagents: true}))
	subs, _ := m.ListSubkeys(ctx, SessionKey{ProjectKey: "-work-app", SessionID: mutID})
	assert.Empty(t, subs)

	// Directory selects by the CLI's name for it.
	m = NewMemorySessionStore()
	require.NoError(t, ImportSessionToStore(ctx, mutID, m, ImportOptions{ProjectsDir: projects, Directory: "/work/app"}))
	got, _ := m.Load(ctx, SessionKey{ProjectKey: "-work-app", SessionID: mutID})
	assert.Len(t, got, 3)

	err := ImportSessionToStore(ctx, mutID, m, ImportOptions{ProjectsDir: projects, Directory: "/elsewhere"})
	assert.ErrorIs(t, err, ErrSessionFileNotFound)
	assert.ErrorIs(t, ImportSessionToStore(ctx, "not-a-uuid", m, ImportOptions{ProjectsDir: projects}), ErrInvalidSessionID)
	assert.ErrorIs(t, ImportSessionToStore(ctx, "99999999-2222-4333-8444-555555555555", m, ImportOptions{ProjectsDir: projects}), ErrSessionFileNotFound)

	// An empty transcript is not a session; a bad line stops the import.
	require.NoError(t, os.WriteFile(filepath.Join(project, mutID+".jsonl"), nil, 0o644))
	assert.ErrorIs(t, ImportSessionToStore(ctx, mutID, m, ImportOptions{ProjectsDir: projects}), ErrSessionFileNotFound)
	require.NoError(t, os.WriteFile(filepath.Join(project, mutID+".jsonl"), []byte("{\"type\":\"user\",\"uuid\":\"x\"}\n[1]\n"), 0o644))
	assert.Error(t, ImportSessionToStore(ctx, mutID, NewMemorySessionStore(), ImportOptions{ProjectsDir: projects}))
}
