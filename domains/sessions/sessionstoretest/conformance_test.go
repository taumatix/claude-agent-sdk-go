package sessionstoretest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/taumatix/claude-agent-sdk-go/domains/sessions"
	"github.com/taumatix/claude-agent-sdk-go/domains/sessions/sessionstoretest"
)

func TestMemorySessionStoreConforms(t *testing.T) {
	sessionstoretest.Run(t, func(*testing.T) sessions.SessionStore { return sessions.NewMemorySessionStore() })
}

// required-only store: optional contracts must skip, required ones must pass.
type bare struct{ m *sessions.MemorySessionStore }

func (b bare) Append(ctx context.Context, k sessions.SessionKey, e []sessions.SessionStoreEntry) error {
	return b.m.Append(ctx, k, e)
}
func (b bare) Load(ctx context.Context, k sessions.SessionKey) ([]sessions.SessionStoreEntry, error) {
	return b.m.Load(ctx, k)
}

func TestARequiredOnlyStoreConforms(t *testing.T) {
	sessionstoretest.Run(t, func(*testing.T) sessions.SessionStore { return bare{sessions.NewMemorySessionStore()} })
}

// brokenStore ignores the subpath, so it must fail the contracts that depend on it.
type brokenStore struct{ *sessions.MemorySessionStore }

func (b brokenStore) Append(ctx context.Context, k sessions.SessionKey, e []sessions.SessionStoreEntry) error {
	k.Subpath = ""
	return b.MemorySessionStore.Append(ctx, k, e)
}

// reversing store breaks ordering.
type reversing struct{ *sessions.MemorySessionStore }

func (r reversing) Load(ctx context.Context, k sessions.SessionKey) ([]sessions.SessionStoreEntry, error) {
	es, err := r.MemorySessionStore.Load(ctx, k)
	for i, j := 0, len(es)-1; i < j; i, j = i+1, j-1 {
		es[i], es[j] = es[j], es[i]
	}
	return es, err
}

// staleSummaries stamps sidecars in epoch seconds, off the clock ListSessions uses.
type staleSummaries struct{ *sessions.MemorySessionStore }

func (s staleSummaries) ListSessionSummaries(ctx context.Context, p string) ([]sessions.SessionSummaryEntry, error) {
	got, err := s.MemorySessionStore.ListSessionSummaries(ctx, p)
	for i := range got {
		got[i].MtimeMs /= 1000
	}
	return got, err
}

func TestBrokenStoresAreCaught(t *testing.T) {
	cases := map[string]struct {
		store sessions.SessionStore
		want  string
	}{
		"subpath ignored": {brokenStore{sessions.NewMemorySessionStore()}, "a subpath is stored apart"},
		"order reversed":  {reversing{sessions.NewMemorySessionStore()}, "append then load"},
		"summary clock":   {staleSummaries{sessions.NewMemorySessionStore()}, "ListSessionSummaries"},
	}
	for name, c := range cases {
		failed := []string{}
		for _, ct := range sessionstoretest.Contracts() {
			if err := ct.Check(context.Background(), func() sessions.SessionStore {
				switch s := c.store.(type) {
				case brokenStore:
					return brokenStore{sessions.NewMemorySessionStore()}
				case reversing:
					_ = s
					return reversing{sessions.NewMemorySessionStore()}
				case staleSummaries:
					return staleSummaries{sessions.NewMemorySessionStore()}
				}
				return nil
			}); err != nil {
				failed = append(failed, ct.Name)
			}
		}
		found := false
		for _, f := range failed {
			found = found || strings.HasPrefix(f, c.want)
		}
		if !found {
			t.Errorf("%s: contract %q did not fail; failed = %v", name, c.want, failed)
		}
	}
}
