package sessions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func subagentStore(t *testing.T) *MemorySessionStore {
	m := NewMemorySessionStore()
	ctx := context.Background()
	put := func(sub string, entries ...SessionStoreEntry) {
		require.NoError(t, m.Append(ctx, SessionKey{ProjectKey: "p", SessionID: mutID, Subpath: sub}, entries))
	}
	msg := func(typ, uuid, parent string) SessionStoreEntry {
		e := SessionStoreEntry{"type": typ, "uuid": uuid, "sessionId": mutID, "message": map[string]any{"role": typ, "content": uuid}}
		if parent != "" {
			e["parentUuid"] = parent
		}
		return e
	}
	put("subagents/agent-a1",
		SessionStoreEntry{"type": "agent_metadata", "toolUseId": "toolu_1", "parentAgentId": "root"},
		msg("user", "u1", ""), msg("assistant", "a1", "u1"), msg("user", "u2", "a1"), msg("assistant", "a2", "u2"))
	put("subagents/workflows/run1/agent-b2",
		msg("user", "x1", ""), msg("assistant", "x2", "x1"))
	put("subagents/agent-cyc", msg("user", "c1", "c2"), msg("assistant", "c2", "c1"))
	// The main transcript is not a subagent.
	require.NoError(t, m.Append(ctx, SessionKey{ProjectKey: "p", SessionID: mutID}, []SessionStoreEntry{msg("user", "m1", "")}))
	return m
}

func TestListSubagentsFromStoreFindsFlatAndNestedOnes(t *testing.T) {
	ids, err := ListSubagentsFromStore(context.Background(), subagentStore(t), "p", mutID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a1", "b2", "cyc"}, ids)

	none, err := ListSubagentsFromStore(context.Background(), subagentStore(t), "p", "not-a-uuid")
	require.NoError(t, err)
	assert.Empty(t, none)

	_, err = ListSubagentsFromStore(context.Background(), loadOnlyStore{subagentStore(t)}, "p", mutID)
	assert.ErrorIs(t, err, ErrSubkeysUnsupported)
}

func TestGetSubagentMessagesFromStoreReadsTheChainWithItsParents(t *testing.T) {
	ctx, m := context.Background(), subagentStore(t)
	msgs, err := GetSubagentMessagesFromStore(ctx, m, "p", mutID, "a1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"u1", "a1", "u2", "a2"}, uuids(msgs))
	for _, mm := range msgs {
		assert.Equal(t, "toolu_1", mm.ParentToolUseID)
		assert.Equal(t, "root", mm.ParentAgentID)
	}

	page, err := GetSubagentMessagesFromStore(ctx, m, "p", mutID, "a1", 2, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"a1", "u2"}, uuids(page))

	nested, err := GetSubagentMessagesFromStore(ctx, m, "p", mutID, "b2", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"x1", "x2"}, uuids(nested))
	assert.Nil(t, nested[0].ParentToolUseID, "no metadata entry, no parent ids")

	cyc, err := GetSubagentMessagesFromStore(ctx, m, "p", mutID, "cyc", 0, 0)
	require.NoError(t, err)
	assert.Len(t, cyc, 2, "a parent cycle ends the walk")

	for _, id := range []string{"missing", ""} {
		got, err := GetSubagentMessagesFromStore(ctx, m, "p", mutID, id, 0, 0)
		require.NoError(t, err)
		assert.Empty(t, got)
	}
}

// A store that cannot list sub-transcripts is asked for the direct path, so only a
// top-level subagent is found.
func TestGetSubagentMessagesFromStoreWithoutASubkeyListerTriesTheDirectPath(t *testing.T) {
	ctx, st := context.Background(), loadOnlyStore{subagentStore(t)}
	direct, err := GetSubagentMessagesFromStore(ctx, st, "p", mutID, "a1", 0, 0)
	require.NoError(t, err)
	assert.Len(t, direct, 4)
	nested, err := GetSubagentMessagesFromStore(ctx, st, "p", mutID, "b2", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, nested)
}
