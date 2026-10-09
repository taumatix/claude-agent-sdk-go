// Package sessionstoretest is the conformance suite for sessions.SessionStore adapters, a
// port of upstream's session_store_conformance (all 14 contracts; the 14th, summaries, runs
// only for a store that implements sessions.SessionSummaryLister).
//
//	func TestMyStore(t *testing.T) {
//		sessionstoretest.Run(t, func(t *testing.T) sessions.SessionStore { return newMyStore(t) })
//	}
//
// Contracts for the optional interfaces run only when the store implements them.
package sessionstoretest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/taumatix/claude-agent-sdk-go/domains/sessions"
)

// Contract is one behaviour every adapter must have. Check returns nil when the store
// honours it; it is exported so a harness for another test framework, or a test that a
// broken store is caught, can drive it without a *testing.T.
type Contract struct {
	Name string
	// Optional is "" for a required contract, else the interface name it needs.
	Optional string
	Check    func(ctx context.Context, newStore func() sessions.SessionStore) error
}

// Run runs every contract against stores from newStore, each on a fresh store, as subtests.
func Run(t *testing.T, newStore func(t *testing.T) sessions.SessionStore) {
	t.Helper()
	for _, c := range Contracts() {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			probe := newStore(t)
			if skip := missing(probe, c.Optional); skip != "" {
				t.Skipf("store does not implement %s", skip)
			}
			if err := c.Check(context.Background(), func() sessions.SessionStore { return newStore(t) }); err != nil {
				t.Error(err)
			}
		})
	}
}

func missing(s sessions.SessionStore, optional string) string {
	ok := true
	switch optional {
	case "SessionLister":
		_, ok = s.(sessions.SessionLister)
	case "SessionDeleter":
		_, ok = s.(sessions.SessionDeleter)
	case "SubkeyLister":
		_, ok = s.(sessions.SubkeyLister)
	case "SessionSummaryLister":
		_, ok = s.(sessions.SessionSummaryLister)
	}
	if ok {
		return ""
	}
	return optional
}

var key = sessions.SessionKey{ProjectKey: "proj", SessionID: "sess"}

func entry(kv ...any) sessions.SessionStoreEntry {
	e := sessions.SessionStoreEntry{"type": "x"}
	for i := 0; i+1 < len(kv); i += 2 {
		e[kv[i].(string)] = kv[i+1]
	}
	return e
}

// same compares as JSON, so a store that returns numbers as float64 is not penalised.
func same(a, b []sessions.SessionStoreEntry) bool {
	norm := func(x []sessions.SessionStoreEntry) any {
		raw, err := json.Marshal(x)
		if err != nil {
			return err.Error()
		}
		var v any
		_ = json.Unmarshal(raw, &v)
		return v
	}
	return reflect.DeepEqual(norm(a), norm(b))
}

func wantLoad(ctx context.Context, s sessions.SessionStore, k sessions.SessionKey, want []sessions.SessionStoreEntry) error {
	got, err := s.Load(ctx, k)
	if err != nil {
		return fmt.Errorf("Load(%+v): %w", k, err)
	}
	if want == nil {
		if got != nil {
			return fmt.Errorf("Load(%+v) = %v, want nil for a key never written", k, got)
		}
		return nil
	}
	if !same(got, want) {
		return fmt.Errorf("Load(%+v) = %v, want %v", k, got, want)
	}
	return nil
}

func app(ctx context.Context, s sessions.SessionStore, k sessions.SessionKey, es ...sessions.SessionStoreEntry) error {
	if err := s.Append(ctx, k, es); err != nil {
		return fmt.Errorf("Append(%+v): %w", k, err)
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// Contracts returns the suite.
func Contracts() []Contract {
	sub1 := sessions.SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-1"}
	sub2 := sessions.SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-2"}
	return []Contract{
		{Name: "append then load returns the entries in order", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			return firstErr(app(ctx, s, key, entry("uuid", "b", "n", 1), entry("uuid", "a", "n", 2)),
				wantLoad(ctx, s, key, []sessions.SessionStoreEntry{entry("uuid", "b", "n", 1), entry("uuid", "a", "n", 2)}))
		}},
		{Name: "load of an unknown key returns nil", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			return firstErr(wantLoad(ctx, s, sessions.SessionKey{ProjectKey: "proj", SessionID: "nope"}, nil),
				app(ctx, s, key, entry("uuid", "x")),
				wantLoad(ctx, s, sessions.SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "nope"}, nil))
		}},
		{Name: "several appends keep call order", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			return firstErr(app(ctx, s, key, entry("uuid", "z")),
				app(ctx, s, key, entry("uuid", "a"), entry("uuid", "m")),
				app(ctx, s, key, entry("uuid", "b")),
				wantLoad(ctx, s, key, []sessions.SessionStoreEntry{entry("uuid", "z"), entry("uuid", "a"), entry("uuid", "m"), entry("uuid", "b")}))
		}},
		{Name: "appending nothing changes nothing", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			return firstErr(app(ctx, s, key, entry("uuid", "a")), app(ctx, s, key),
				wantLoad(ctx, s, key, []sessions.SessionStoreEntry{entry("uuid", "a")}))
		}},
		{Name: "a subpath is stored apart from the main transcript", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			return firstErr(app(ctx, s, key, entry("uuid", "m")), app(ctx, s, sub1, entry("uuid", "s")),
				wantLoad(ctx, s, key, []sessions.SessionStoreEntry{entry("uuid", "m")}),
				wantLoad(ctx, s, sub1, []sessions.SessionStoreEntry{entry("uuid", "s")}))
		}},
		{Name: "project keys are isolated", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			a := sessions.SessionKey{ProjectKey: "A", SessionID: "s1"}
			b := sessions.SessionKey{ProjectKey: "B", SessionID: "s1"}
			err := firstErr(app(ctx, s, a, entry("from", "A")), app(ctx, s, b, entry("from", "B")),
				wantLoad(ctx, s, a, []sessions.SessionStoreEntry{entry("from", "A")}),
				wantLoad(ctx, s, b, []sessions.SessionStoreEntry{entry("from", "B")}))
			if l, ok := s.(sessions.SessionLister); err == nil && ok {
				for _, p := range []string{"A", "B"} {
					got, lerr := l.ListSessions(ctx, p)
					if lerr != nil || len(got) != 1 {
						return fmt.Errorf("ListSessions(%q) = %v, %v; want one session", p, got, lerr)
					}
				}
			}
			return err
		}},
		{Name: "ListSessions returns the project's sessions with epoch-millisecond mtimes", Optional: "SessionLister", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			l := s.(sessions.SessionLister)
			if err := firstErr(app(ctx, s, sessions.SessionKey{ProjectKey: "proj", SessionID: "a"}, entry("n", 1)),
				app(ctx, s, sessions.SessionKey{ProjectKey: "proj", SessionID: "b"}, entry("n", 1)),
				app(ctx, s, sessions.SessionKey{ProjectKey: "other", SessionID: "c"}, entry("n", 1))); err != nil {
				return err
			}
			got, err := l.ListSessions(ctx, "proj")
			if err != nil {
				return err
			}
			ids := []string{}
			for _, e := range got {
				ids = append(ids, e.SessionID)
				if e.MtimeMs <= 1e12 {
					return fmt.Errorf("mtime %d for %q is not epoch milliseconds", e.MtimeMs, e.SessionID)
				}
			}
			sort.Strings(ids)
			if !reflect.DeepEqual(ids, []string{"a", "b"}) {
				return fmt.Errorf("ListSessions(proj) ids = %v, want [a b]", ids)
			}
			none, err := l.ListSessions(ctx, "never-appended-project")
			if err != nil || len(none) != 0 {
				return fmt.Errorf("ListSessions of an unknown project = %v, %v; want empty", none, err)
			}
			return nil
		}},
		{Name: "ListSessions leaves out sub-transcripts", Optional: "SessionLister", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			main := sessions.SessionKey{ProjectKey: "proj", SessionID: "main"}
			sub := sessions.SessionKey{ProjectKey: "proj", SessionID: "main", Subpath: "subagents/agent-1"}
			if err := firstErr(app(ctx, s, main, entry("n", 1)), app(ctx, s, sub, entry("n", 1))); err != nil {
				return err
			}
			got, err := s.(sessions.SessionLister).ListSessions(ctx, "proj")
			if err != nil || len(got) != 1 || got[0].SessionID != "main" {
				return fmt.Errorf("ListSessions = %v, %v; want only main", got, err)
			}
			return nil
		}},
		{Name: "ListSessionSummaries returns summaries that fold again", Optional: "SessionSummaryLister", Check: checkSummaries},
		{Name: "deleting the main transcript makes it load as nil", Optional: "SessionDeleter", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			d := s.(sessions.SessionDeleter)
			if err := d.Delete(ctx, sessions.SessionKey{ProjectKey: "proj", SessionID: "never-written"}); err != nil {
				return fmt.Errorf("deleting a key never written: %w", err)
			}
			if err := app(ctx, s, key, entry("n", 1)); err != nil {
				return err
			}
			if err := d.Delete(ctx, key); err != nil {
				return err
			}
			return wantLoad(ctx, s, key, nil)
		}},
		{Name: "deleting the main transcript removes its sub-transcripts and nothing else", Optional: "SessionDeleter", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			other := sessions.SessionKey{ProjectKey: "proj", SessionID: "sess2"}
			otherProj := sessions.SessionKey{ProjectKey: "other-proj", SessionID: "sess"}
			one := []sessions.SessionStoreEntry{entry("n", 1)}
			if err := firstErr(app(ctx, s, key, one...), app(ctx, s, sub1, one...), app(ctx, s, sub2, one...),
				app(ctx, s, other, one...), app(ctx, s, otherProj, one...),
				s.(sessions.SessionDeleter).Delete(ctx, key)); err != nil {
				return err
			}
			err := firstErr(wantLoad(ctx, s, key, nil), wantLoad(ctx, s, sub1, nil), wantLoad(ctx, s, sub2, nil),
				wantLoad(ctx, s, other, one), wantLoad(ctx, s, otherProj, one))
			if l, ok := s.(sessions.SubkeyLister); err == nil && ok {
				if got, lerr := l.ListSubkeys(ctx, key); lerr != nil || len(got) != 0 {
					return fmt.Errorf("ListSubkeys after delete = %v, %v; want none", got, lerr)
				}
			}
			if l, ok := s.(sessions.SessionLister); err == nil && ok {
				got, _ := l.ListSessions(ctx, key.ProjectKey)
				for _, e := range got {
					if e.SessionID == key.SessionID {
						return fmt.Errorf("the deleted session is still listed by ListSessions")
					}
				}
			}
			return err
		}},
		{Name: "deleting a subpath removes only that one", Optional: "SessionDeleter", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			one := []sessions.SessionStoreEntry{entry("n", 1)}
			if err := firstErr(app(ctx, s, key, one...), app(ctx, s, sub1, one...), app(ctx, s, sub2, one...),
				s.(sessions.SessionDeleter).Delete(ctx, sub1)); err != nil {
				return err
			}
			err := firstErr(wantLoad(ctx, s, sub1, nil), wantLoad(ctx, s, sub2, one), wantLoad(ctx, s, key, one))
			if l, ok := s.(sessions.SubkeyLister); err == nil && ok {
				got, _ := l.ListSubkeys(ctx, key)
				if !reflect.DeepEqual(got, []string{"subagents/agent-2"}) {
					return fmt.Errorf("ListSubkeys = %v, want [subagents/agent-2]", got)
				}
			}
			return err
		}},
		{Name: "ListSubkeys returns this session's subpaths", Optional: "SubkeyLister", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			one := []sessions.SessionStoreEntry{entry("n", 1)}
			if err := firstErr(app(ctx, s, key, one...), app(ctx, s, sub1, one...), app(ctx, s, sub2, one...),
				app(ctx, s, sessions.SessionKey{ProjectKey: "proj", SessionID: "other-sess", Subpath: "subagents/agent-x"}, one...)); err != nil {
				return err
			}
			got, err := s.(sessions.SubkeyLister).ListSubkeys(ctx, key)
			if err != nil {
				return err
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, []string{"subagents/agent-1", "subagents/agent-2"}) {
				return fmt.Errorf("ListSubkeys = %v, want agent-1 and agent-2 only", got)
			}
			return nil
		}},
		{Name: "ListSubkeys leaves out the main transcript", Optional: "SubkeyLister", Check: func(ctx context.Context, n func() sessions.SessionStore) error {
			s := n()
			l := s.(sessions.SubkeyLister)
			if err := app(ctx, s, key, entry("n", 1)); err != nil {
				return err
			}
			got, err := l.ListSubkeys(ctx, key)
			if err != nil || len(got) != 0 {
				return fmt.Errorf("ListSubkeys of a session with no sub-transcripts = %v, %v; want none", got, err)
			}
			got, err = l.ListSubkeys(ctx, sessions.SessionKey{ProjectKey: "proj", SessionID: "never-appended"})
			if err != nil || len(got) != 0 {
				return fmt.Errorf("ListSubkeys of an unknown session = %v, %v; want none", got, err)
			}
			return nil
		}},
	}
}

func checkSummaries(ctx context.Context, n func() sessions.SessionStore) error {
	s := n()
	l := s.(sessions.SessionSummaryLister)
	k := sessions.SessionKey{ProjectKey: "proj", SessionID: "summ-sess"}
	if err := firstErr(
		app(ctx, s, k,
			entry("timestamp", "2024-01-01T00:00:00.000Z", "customTitle", "first"),
			entry("timestamp", "2024-01-01T00:00:01.000Z")),
		app(ctx, s, k, entry("timestamp", "2024-01-01T00:00:02.000Z", "customTitle", "second")),
		app(ctx, s, sessions.SessionKey{ProjectKey: "other", SessionID: "elsewhere"},
			entry("timestamp", "2024-01-01T00:00:00.000Z")),
	); err != nil {
		return err
	}
	got, err := l.ListSessionSummaries(ctx, "proj")
	if err != nil {
		return err
	}
	if len(got) != 1 || got[0].SessionID != "summ-sess" {
		return fmt.Errorf("ListSessionSummaries(proj) = %v, want only summ-sess", got)
	}
	sum := got[0]
	if sum.MtimeMs < 1e12 {
		return fmt.Errorf("summary mtime %d is not epoch milliseconds", sum.MtimeMs)
	}
	if ls, ok := s.(sessions.SessionLister); ok {
		list, _ := ls.ListSessions(ctx, "proj")
		for _, e := range list {
			if e.SessionID == "summ-sess" && sum.MtimeMs < e.MtimeMs {
				return fmt.Errorf("summary mtime %d is older than ListSessions mtime %d: the two must share a clock", sum.MtimeMs, e.MtimeMs)
			}
		}
	}
	if sum.Data == nil {
		return fmt.Errorf("summary Data is nil")
	}
	if info, ok := sessions.SummaryToSessionInfo(sum, ""); !ok || info.CustomTitle != "second" {
		return fmt.Errorf("summary folded to %+v, %v; want custom title %q", info, ok, "second")
	}
	re := sessions.FoldSessionSummary(&sum, k, []sessions.SessionStoreEntry{entry("timestamp", "2024-01-01T00:00:03.000Z")})
	if re.SessionID != "summ-sess" || re.MtimeMs != sum.MtimeMs {
		return fmt.Errorf("refolding changed the id or mtime: %+v from %+v", re, sum)
	}
	sub := sessions.SessionKey{ProjectKey: "proj", SessionID: "summ-sess", Subpath: "subagents/agent-1"}
	if err := app(ctx, s, sub, entry("timestamp", "2024-01-01T00:00:09.000Z", "customTitle", "subagent")); err != nil {
		return err
	}
	after, err := l.ListSessionSummaries(ctx, "proj")
	if err != nil {
		return err
	}
	if len(after) != 1 || !reflect.DeepEqual(after[0].Data, sum.Data) {
		return fmt.Errorf("a sub-transcript append changed the main summary: %v -> %v", sum.Data, after)
	}
	if none, _ := l.ListSessionSummaries(ctx, "never-appended-project"); len(none) != 0 {
		return fmt.Errorf("an unknown project has summaries: %v", none)
	}
	if d, ok := s.(sessions.SessionDeleter); ok {
		if err := d.Delete(ctx, k); err != nil {
			return err
		}
		if gone, _ := l.ListSessionSummaries(ctx, "proj"); len(gone) != 0 {
			return fmt.Errorf("a deleted session still has a summary: %v", gone)
		}
	}
	return nil
}
