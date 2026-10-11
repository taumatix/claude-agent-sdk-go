package sessions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fkA = "aaaaaaaa-0000-4000-8000-000000000001"
	fkP = "aaaaaaaa-0000-4000-8000-000000000002" // progress
	fkB = "aaaaaaaa-0000-4000-8000-000000000003"
	fkS = "aaaaaaaa-0000-4000-8000-000000000004" // sidechain
	fkC = "aaaaaaaa-0000-4000-8000-000000000005"
)

func seedFork(t *testing.T, st SessionStore) {
	t.Helper()
	require.NoError(t, st.Append(context.Background(), SessionKey{ProjectKey: "p", SessionID: mutID}, []SessionStoreEntry{
		{"type": "user", "uuid": fkA, "sessionId": mutID, "timestamp": "2026-10-01T00:00:00.000Z", "slug": "s",
			"message": map[string]any{"role": "user", "content": "first prompt"}},
		{"type": "progress", "uuid": fkP, "parentUuid": fkA, "sessionId": mutID, "timestamp": "2026-10-01T00:00:01.000Z"},
		{"type": "assistant", "uuid": fkB, "parentUuid": fkP, "sessionId": mutID, "timestamp": "2026-10-01T00:00:02.000Z",
			"message": map[string]any{"role": "assistant", "content": "hi"}},
		{"type": "user", "uuid": fkS, "parentUuid": fkB, "sessionId": mutID, "isSidechain": true, "timestamp": "2026-10-01T00:00:03.000Z"},
		{"type": "system", "uuid": fkC, "parentUuid": fkB, "logicalParentUuid": fkA, "sessionId": mutID, "timestamp": "2026-10-01T00:00:04.000Z"},
		{"type": "content-replacement", "sessionId": mutID, "replacements": []any{map[string]any{"k": "v"}}},
	}))
}

func forkStores(t *testing.T) map[string]SessionStore {
	return map[string]SessionStore{"memory": NewMemorySessionStore(), "file": NewFileSessionStore(t.TempDir())}
}

// Driven through both real adapters and read back through the listing and the message
// reader, the way a caller would use a fork.
func TestForkSessionViaStoreWritesAFreshSessionAKnownStoreCanListAndRead(t *testing.T) {
	ctx := context.Background()
	for name, st := range forkStores(t) {
		t.Run(name, func(t *testing.T) {
			seedFork(t, st)
			id, err := ForkSessionViaStore(ctx, st, "p", mutID, ForkOptions{})
			require.NoError(t, err)
			require.NotEqual(t, mutID, id)
			require.Regexp(t, sessionIDRE, id)

			got, err := st.Load(ctx, SessionKey{ProjectKey: "p", SessionID: id})
			require.NoError(t, err)
			require.Len(t, got, 5, "user, assistant, system, content-replacement, custom-title: no progress, no sidechain")

			user, asst, sys := got[0], got[1], got[2]
			assert.NotEqual(t, fkA, user["uuid"])
			assert.Nil(t, user["parentUuid"])
			assert.Equal(t, id, user["sessionId"])
			assert.Equal(t, map[string]any{"sessionId": mutID, "messageUuid": fkA}, user["forkedFrom"])
			assert.NotContains(t, user, "slug")
			assert.Equal(t, "2026-10-01T00:00:00.000Z", user["timestamp"], "only the last entry is re-stamped")
			assert.Equal(t, user["uuid"], asst["parentUuid"], "the chain skips the progress entry")
			assert.Equal(t, asst["uuid"], sys["parentUuid"])
			assert.Equal(t, user["uuid"], sys["logicalParentUuid"])
			assert.NotEqual(t, "2026-10-01T00:00:04.000Z", sys["timestamp"])
			assert.Equal(t, "content-replacement", got[3]["type"])
			assert.Equal(t, "first prompt (fork)", got[4]["customTitle"])

			info, err := GetSessionInfoFromStore(ctx, st, "p", "/proj", id)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, "first prompt (fork)", info.Summary)
			msgs, err := GetSessionMessagesFromStore(ctx, st, "p", id, 0, 0)
			require.NoError(t, err)
			assert.Len(t, msgs, 2)

			src, err := st.Load(ctx, SessionKey{ProjectKey: "p", SessionID: mutID})
			require.NoError(t, err)
			assert.Len(t, src, 6, "the source is untouched")
		})
	}
}

func TestForkSessionViaStoreCutsAtAMessageAndTakesATitle(t *testing.T) {
	ctx, st := context.Background(), NewMemorySessionStore()
	seedFork(t, st)
	id, err := ForkSessionViaStore(ctx, st, "p", mutID, ForkOptions{UpToMessageID: fkB, Title: "  branch  "})
	require.NoError(t, err)
	got, _ := st.Load(ctx, SessionKey{ProjectKey: "p", SessionID: id})
	require.Len(t, got, 4, "user, assistant, content-replacement, custom-title")
	assert.Equal(t, "branch", got[3]["customTitle"])
}

func TestForkSessionViaStoreRefusals(t *testing.T) {
	ctx, st := context.Background(), NewMemorySessionStore()
	seedFork(t, st)
	_, err := ForkSessionViaStore(ctx, st, "p", "nope", ForkOptions{})
	assert.ErrorIs(t, err, ErrInvalidSessionID)
	_, err = ForkSessionViaStore(ctx, st, "p", mutID, ForkOptions{UpToMessageID: "nope"})
	assert.ErrorIs(t, err, ErrInvalidSessionID)
	_, err = ForkSessionViaStore(ctx, st, "p", "bbbbbbbb-0000-4000-8000-000000000001", ForkOptions{})
	assert.ErrorIs(t, err, ErrSessionNotFound)
	_, err = ForkSessionViaStore(ctx, st, "p", mutID, ForkOptions{UpToMessageID: "cccccccc-0000-4000-8000-000000000001"})
	assert.ErrorContains(t, err, "not found")
}
