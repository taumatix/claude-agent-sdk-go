package sessions

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingStore struct {
	*MemorySessionStore
	loads atomic.Int64
}

func (c *countingStore) Load(ctx context.Context, k SessionKey) ([]SessionStoreEntry, error) {
	c.loads.Add(1)
	return c.MemorySessionStore.Load(ctx, k)
}

// plainStore hides the optional interfaces other than Append/Load/ListSessions.
type plainStore struct{ m *MemorySessionStore }

func (p plainStore) Append(c context.Context, k SessionKey, e []SessionStoreEntry) error {
	return p.m.Append(c, k, e)
}
func (p plainStore) Load(c context.Context, k SessionKey) ([]SessionStoreEntry, error) {
	return p.m.Load(c, k)
}
func (p plainStore) ListSessions(c context.Context, pk string) ([]SessionListEntry, error) {
	return p.m.ListSessions(c, pk)
}

type loadOnlyStore struct{ m *MemorySessionStore }

func (l loadOnlyStore) Append(c context.Context, k SessionKey, e []SessionStoreEntry) error {
	return l.m.Append(c, k, e)
}
func (l loadOnlyStore) Load(c context.Context, k SessionKey) ([]SessionStoreEntry, error) {
	return l.m.Load(c, k)
}

func seed(t *testing.T, m *MemorySessionStore, n int) {
	for i := 0; i < n; i++ {
		k := SessionKey{ProjectKey: "p", SessionID: fmt.Sprintf("s%02d", i)}
		require.NoError(t, m.Append(context.Background(), k, []SessionStoreEntry{user(fmt.Sprintf("prompt %d", i))}))
	}
}

func TestListSessionsFromStoreFreshSummariesNeedNoLoad(t *testing.T) {
	cs := &countingStore{MemorySessionStore: NewMemorySessionStore()}
	seed(t, cs.MemorySessionStore, 5)
	got, err := ListSessionsFromStore(context.Background(), cs, "p", "/proj", 0, 0)
	require.NoError(t, err)
	assert.Len(t, got, 5)
	assert.Zero(t, cs.loads.Load())
	assert.Equal(t, "/proj", got[0].Cwd)
	for i := 1; i < len(got); i++ {
		assert.GreaterOrEqual(t, got[i-1].LastModified, got[i].LastModified)
	}
}

func TestListSessionsFromStorePaginationAndSidechain(t *testing.T) {
	m := NewMemorySessionStore()
	seed(t, m, 6)
	require.NoError(t, m.Append(context.Background(), SessionKey{ProjectKey: "p", SessionID: "side"},
		[]SessionStoreEntry{user("x", "isSidechain", true)}))
	all, err := ListSessionsFromStore(context.Background(), m, "p", "", 0, 0)
	require.NoError(t, err)
	assert.Len(t, all, 6)
	page, err := ListSessionsFromStore(context.Background(), m, "p", "", 2, 1)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, all[1].SessionID, page[0].SessionID)
	none, err := ListSessionsFromStore(context.Background(), m, "p", "", 3, 99)
	require.NoError(t, err)
	assert.Empty(t, none)
}

// summaryStoreWith serves a chosen set of summaries over a real store's sessions.
type summaryStoreWith struct {
	*countingStore
	sums []SessionSummaryEntry
}

func (s summaryStoreWith) ListSessionSummaries(context.Context, string) ([]SessionSummaryEntry, error) {
	return s.sums, nil
}

func TestListSessionsFromStoreStaleAndMissingSummariesAreRefolded(t *testing.T) {
	cs := &countingStore{MemorySessionStore: NewMemorySessionStore()}
	seed(t, cs.MemorySessionStore, 4)
	real, _ := cs.MemorySessionStore.ListSessionSummaries(context.Background(), "p")
	listing, _ := cs.MemorySessionStore.ListSessions(context.Background(), "p")
	mt := map[string]int64{}
	for _, l := range listing {
		mt[l.SessionID] = l.MtimeMs
	}
	var sums []SessionSummaryEntry
	for _, s := range real {
		switch s.SessionID {
		case "s00": // missing: dropped
			continue
		case "s01": // stale: mtime behind the listing, content wrong
			s.MtimeMs = mt["s01"] - 1
			s.Data = map[string]any{"first_prompt": "STALE", "first_prompt_locked": true}
		}
		sums = append(sums, s)
	}
	sums = append(sums, SessionSummaryEntry{SessionID: "gone", MtimeMs: 1, Data: map[string]any{"custom_title": "gone"}})
	st := summaryStoreWith{cs, sums}
	got, err := ListSessionsFromStore(context.Background(), st, "p", "", 0, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 2, cs.loads.Load())
	by := map[string]SessionInfo{}
	for _, g := range got {
		by[g.SessionID] = g
	}
	assert.Len(t, by, 4)
	assert.Equal(t, "prompt 1", by["s01"].Summary)
	assert.Equal(t, "prompt 0", by["s00"].Summary)
	assert.NotContains(t, by, "gone")
}

func TestListSessionsFromStoreGapFillIsBoundedByPage(t *testing.T) {
	cs := &countingStore{MemorySessionStore: NewMemorySessionStore()}
	seed(t, cs.MemorySessionStore, 10)
	st := summaryStoreWith{cs, nil}
	got, err := ListSessionsFromStore(context.Background(), st, "p", "", 3, 0)
	require.NoError(t, err)
	assert.Len(t, got, 3)
	assert.EqualValues(t, 3, cs.loads.Load())
}

func TestListSessionsFromStoreSlowPath(t *testing.T) {
	m := NewMemorySessionStore()
	seed(t, m, 20)
	got, err := ListSessionsFromStore(context.Background(), plainStore{m}, "p", "", 5, 0)
	require.NoError(t, err)
	assert.Len(t, got, 5)
	assert.Equal(t, "prompt 19", got[0].Summary)
}

func TestListSessionsFromStoreNeedsAListing(t *testing.T) {
	_, err := ListSessionsFromStore(context.Background(), loadOnlyStore{}, "p", "", 0, 0)
	assert.Error(t, err)
}

func TestGetSessionInfoFromStore(t *testing.T) {
	ctx := context.Background()
	m := NewMemorySessionStore()
	key := SessionKey{ProjectKey: "p", SessionID: "s1"}
	require.NoError(t, m.Append(ctx, key, []SessionStoreEntry{
		user("first prompt", "timestamp", "2026-01-02T03:04:05Z"),
		{"type": "custom-title", "customTitle": "Named", "timestamp": "2026-01-02T03:04:06Z"},
	}))
	side := SessionKey{ProjectKey: "p", SessionID: "side"}
	require.NoError(t, m.Append(ctx, side, []SessionStoreEntry{user("x", "isSidechain", true)}))

	// A store that offers only Append and Load is enough.
	info, err := GetSessionInfoFromStore(ctx, loadOnlyStore{m}, "p", "/work", "s1")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "s1", info.SessionID)
	assert.Equal(t, "Named", info.Summary)
	assert.Equal(t, "first prompt", info.FirstPrompt)
	assert.Equal(t, int64(1767323046000), info.LastModified)

	for name, id := range map[string]string{"missing": "nope", "sidechain": "side"} {
		got, err := GetSessionInfoFromStore(ctx, m, "p", "/work", id)
		require.NoError(t, err, name)
		assert.Nil(t, got, name)
	}
	other, err := GetSessionInfoFromStore(ctx, m, "other", "/work", "s1")
	require.NoError(t, err)
	assert.Nil(t, other, "a session is found only under its own project key")
}

type failingLoad struct{ loadOnlyStore }

func (failingLoad) Load(context.Context, SessionKey) ([]SessionStoreEntry, error) {
	return nil, fmt.Errorf("backend down")
}

func TestGetSessionInfoFromStoreReturnsLoadErrors(t *testing.T) {
	_, err := GetSessionInfoFromStore(context.Background(), failingLoad{}, "p", "/w", "s1")
	assert.EqualError(t, err, "backend down")
}

// The single lookup and the listing derive a session's metadata the same way.
func TestGetSessionInfoFromStoreAgreesWithTheListing(t *testing.T) {
	ctx := context.Background()
	m := NewMemorySessionStore()
	seed(t, m, 3)
	listed, err := ListSessionsFromStore(ctx, m, "p", "/work", 0, 0)
	require.NoError(t, err)
	require.Len(t, listed, 3)
	for _, want := range listed {
		got, err := GetSessionInfoFromStore(ctx, m, "p", "/work", want.SessionID)
		require.NoError(t, err)
		require.NotNil(t, got)
		want.LastModified, got.LastModified = 0, 0
		assert.Equal(t, want, *got)
	}
}
