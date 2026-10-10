package sessions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mutID = "11111111-2222-4333-8444-555555555555"

func seededMemory(t *testing.T) *MemorySessionStore {
	m := NewMemorySessionStore()
	require.NoError(t, m.Append(context.Background(), SessionKey{ProjectKey: "p", SessionID: mutID}, []SessionStoreEntry{
		{"type": "user", "uuid": "a", "sessionId": mutID, "timestamp": "2026-10-01T00:00:00.000Z",
			"message": map[string]any{"role": "user", "content": "first prompt"}},
		{"type": "assistant", "uuid": "b", "parentUuid": "a", "sessionId": mutID, "timestamp": "2026-10-01T00:00:01.000Z",
			"message": map[string]any{"role": "assistant", "content": "hi"}},
	}))
	return m
}

func infoOf(t *testing.T, m *MemorySessionStore) *SessionInfo {
	info, err := GetSessionInfoFromStore(context.Background(), m, "p", "/proj", mutID)
	require.NoError(t, err)
	require.NotNil(t, info)
	return info
}

func TestRenameAndTagViaStoreShowInListingAndInfo(t *testing.T) {
	ctx, m := context.Background(), seededMemory(t)
	require.NoError(t, RenameSessionViaStore(ctx, m, "p", mutID, "  Fix the build  "))
	tag := "release\u200b-blocker"
	require.NoError(t, TagSessionViaStore(ctx, m, "p", mutID, &tag))

	info := infoOf(t, m)
	assert.Equal(t, "Fix the build", info.Summary)
	assert.Equal(t, "release-blocker", info.Tag)

	list, err := ListSessionsFromStore(ctx, m, "p", "/proj", 0, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Fix the build", list[0].Summary)

	require.NoError(t, TagSessionViaStore(ctx, m, "p", mutID, nil))
	assert.Empty(t, infoOf(t, m).Tag, "a nil tag clears it")

	msgs, err := GetSessionMessagesFromStore(ctx, m, "p", mutID, 0, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 2, "the metadata entries are not conversation")
}

func TestRenameAndTagViaStoreRejectBadInput(t *testing.T) {
	ctx, m := context.Background(), seededMemory(t)
	assert.ErrorIs(t, RenameSessionViaStore(ctx, m, "p", "not-a-uuid", "x"), ErrInvalidSessionID)
	assert.Error(t, RenameSessionViaStore(ctx, m, "p", mutID, "   "))
	blank := "\u200b \ue000"
	assert.Error(t, TagSessionViaStore(ctx, m, "p", mutID, &blank))
	assert.ErrorIs(t, TagSessionViaStore(ctx, m, "p", "x", nil), ErrInvalidSessionID)
	entries, _ := m.Load(ctx, SessionKey{ProjectKey: "p", SessionID: mutID})
	assert.Len(t, entries, 2, "a rejected call appends nothing")
}

func TestDeleteSessionViaStore(t *testing.T) {
	ctx, m := context.Background(), seededMemory(t)
	asked, err := DeleteSessionViaStore(ctx, m, "p", mutID)
	require.NoError(t, err)
	assert.True(t, asked)
	entries, err := m.Load(ctx, SessionKey{ProjectKey: "p", SessionID: mutID})
	require.NoError(t, err)
	assert.Empty(t, entries)

	asked, err = DeleteSessionViaStore(ctx, loadOnlyStore{m}, "p", mutID)
	require.NoError(t, err)
	assert.False(t, asked, "an append-only store is left alone")
	_, err = DeleteSessionViaStore(ctx, m, "p", "nope")
	assert.ErrorIs(t, err, ErrInvalidSessionID)
}
