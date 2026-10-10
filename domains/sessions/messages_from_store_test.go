package sessions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func turn(typ, uuid, parent, text string, extra ...any) SessionStoreEntry {
	e := SessionStoreEntry{
		"type": typ, "uuid": uuid, "sessionId": "s1",
		"message": map[string]any{"role": typ, "content": text},
	}
	if parent != "" {
		e["parentUuid"] = parent
	}
	for i := 0; i+1 < len(extra); i += 2 {
		e[extra[i].(string)] = extra[i+1]
	}
	return e
}

func uuids(ms []SessionMessage) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.UUID)
	}
	return out
}

func seedEntries(t *testing.T, entries ...SessionStoreEntry) SessionStore {
	m := NewMemorySessionStore()
	require.NoError(t, m.Append(context.Background(), SessionKey{ProjectKey: "p", SessionID: "s1"}, entries))
	return loadOnlyStore{m}
}

func TestGetSessionMessagesFromStoreReturnsTheChainInOrder(t *testing.T) {
	st := seedEntries(t,
		turn("user", "u1", "", "hi"),
		SessionStoreEntry{"type": "custom-title", "customTitle": "x"},
		turn("assistant", "a1", "u1", "hello"),
		turn("user", "u2", "a1", "more"),
		turn("assistant", "a2", "u2", "ok"),
	)
	got, err := GetSessionMessagesFromStore(context.Background(), st, "p", "s1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"u1", "a1", "u2", "a2"}, uuids(got))
	assert.Equal(t, "user", got[0].Type)
	assert.Equal(t, "s1", got[0].SessionID)
	assert.Equal(t, map[string]any{"role": "user", "content": "hi"}, got[0].Message)
}

func TestGetSessionMessagesFromStoreDropsAbandonedBranchesAndHiddenEntries(t *testing.T) {
	st := seedEntries(t,
		turn("user", "u1", "", "q"),
		turn("assistant", "a1", "u1", "first try"),
		turn("assistant", "a1b", "u1", "second try"), // a rewind: sibling of a1
		turn("system", "sys", "a1b", "note"),
		turn("user", "meta", "sys", "caveat", "isMeta", true),
		turn("user", "u2", "meta", "go on"),
		turn("assistant", "side", "u2", "sidechain", "isSidechain", true),
		turn("assistant", "a2", "u2", "answer"),
	)
	got, err := GetSessionMessagesFromStore(context.Background(), st, "p", "s1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"u1", "a1b", "u2", "a2"}, uuids(got),
		"the abandoned a1, the system and meta entries and the sidechain are not returned")
}

func TestGetSessionMessagesFromStorePaginatesAndHandlesEmpty(t *testing.T) {
	st := seedEntries(t,
		turn("user", "u1", "", "1"), turn("assistant", "a1", "u1", "2"),
		turn("user", "u2", "a1", "3"), turn("assistant", "a2", "u2", "4"),
	)
	ctx := context.Background()
	got, err := GetSessionMessagesFromStore(ctx, st, "p", "s1", 2, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"a1", "u2"}, uuids(got))
	got, err = GetSessionMessagesFromStore(ctx, st, "p", "s1", 0, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"a2"}, uuids(got))

	none, err := GetSessionMessagesFromStore(ctx, st, "p", "absent", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestGetSessionMessagesFromStoreSurvivesAParentCycle(t *testing.T) {
	st := seedEntries(t, turn("user", "x", "y", "a"), turn("assistant", "y", "x", "b"))
	got, err := GetSessionMessagesFromStore(context.Background(), st, "p", "s1", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, got, "a cycle has no terminal, so there is no chain, and it must not hang")
}

func TestGetSessionMessagesFromStoreReturnsLoadErrors(t *testing.T) {
	_, err := GetSessionMessagesFromStore(context.Background(), failingLoad{}, "p", "s1", 0, 0)
	assert.EqualError(t, err, "backend down")
}
