package sessions

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

const storeListLoadConcurrency = 16

// ListSessionsFromStore lists a project's sessions from a SessionStore, newest first.
//
// A store that is a SessionSummaryLister answers in one summary call plus one cheap
// ListSessions enumeration, and Load is called only for a session whose summary is missing
// or older than its listed mtime, and only for sessions inside the requested page. Without
// summaries it calls Load once per session, at most 16 at a time, which on a remote backend
// can be expensive. A store with summaries but no SessionLister cannot gap-fill, so a
// session without a summary is absent from the result.
//
// Sidechain sessions and sessions with nothing to show as a summary are dropped before
// limit and offset apply. A limit <= 0 means no limit, an offset <= 0 means none. A Load
// that fails yields a row with an empty Summary rather than failing the listing. The error
// is non-nil when the store implements neither SessionSummaryLister nor SessionLister, or
// when a listing call fails.
func ListSessionsFromStore(ctx context.Context, store SessionStore, projectKey, projectPath string, limit, offset int) ([]SessionInfo, error) {
	lister, canList := store.(SessionLister)
	if sl, ok := store.(SessionSummaryLister); ok {
		summaries, err := sl.ListSessionSummaries(ctx, projectKey)
		if err != nil {
			return nil, err
		}
		var listing []SessionListEntry
		known := map[string]int64{}
		if canList {
			if listing, err = lister.ListSessions(ctx, projectKey); err != nil {
				return nil, err
			}
			for _, e := range listing {
				known[e.SessionID] = e.MtimeMs
			}
		}
		type slot struct {
			mtime int64
			id    string
			info  *SessionInfo
		}
		var slots []slot
		fresh := map[string]bool{}
		for _, s := range summaries {
			if canList {
				m, ok := known[s.SessionID]
				if !ok || s.MtimeMs < m {
					continue
				}
			}
			fresh[s.SessionID] = true
			if info, ok := SummaryToSessionInfo(s, projectPath); ok {
				slots = append(slots, slot{mtime: s.MtimeMs, id: s.SessionID, info: &info})
			}
		}
		for _, e := range listing {
			if !fresh[e.SessionID] {
				slots = append(slots, slot{mtime: e.MtimeMs, id: e.SessionID})
			}
		}
		sort.SliceStable(slots, func(i, j int) bool {
			if slots[i].mtime != slots[j].mtime {
				return slots[i].mtime > slots[j].mtime
			}
			return slots[i].id < slots[j].id
		})
		page := paginate(slots, limit, offset)
		var fill []SessionListEntry
		for _, s := range page {
			if s.info == nil {
				fill = append(fill, SessionListEntry{SessionID: s.id, MtimeMs: s.mtime})
			}
		}
		filled := deriveViaLoad(ctx, store, projectKey, projectPath, fill)
		out := make([]SessionInfo, 0, len(page))
		for _, s := range page {
			if s.info != nil {
				out = append(out, *s.info)
			} else if info, ok := filled[s.id]; ok {
				out = append(out, info)
			}
		}
		return out, nil
	}
	if !canList {
		return nil, errors.New("sessions: store implements neither SessionSummaryLister nor SessionLister")
	}
	listing, err := lister.ListSessions(ctx, projectKey)
	if err != nil {
		return nil, err
	}
	filled := deriveViaLoad(ctx, store, projectKey, projectPath, listing)
	infos := make([]SessionInfo, 0, len(filled))
	for _, info := range filled {
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].LastModified != infos[j].LastModified {
			return infos[i].LastModified > infos[j].LastModified
		}
		return infos[i].SessionID < infos[j].SessionID
	})
	return paginate(infos, limit, offset), nil
}

func paginate[T any](s []T, limit, offset int) []T {
	if offset > 0 {
		if offset >= len(s) {
			return nil
		}
		s = s[offset:]
	}
	if limit > 0 && limit < len(s) {
		s = s[:limit]
	}
	return s
}

// deriveViaLoad folds each listed session from its stored entries. Sidechain and
// empty sessions are absent from the result; a failed Load gives an empty-Summary row.
func deriveViaLoad(ctx context.Context, store SessionStore, projectKey, projectPath string, listing []SessionListEntry) map[string]SessionInfo {
	out := make(map[string]SessionInfo, len(listing))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, storeListLoadConcurrency)
	for _, e := range listing {
		wg.Add(1)
		sem <- struct{}{}
		go func(e SessionListEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			entries, err := store.Load(ctx, SessionKey{ProjectKey: projectKey, SessionID: e.SessionID})
			var info SessionInfo
			switch {
			case err != nil:
				info = SessionInfo{SessionID: e.SessionID, LastModified: e.MtimeMs}
			case len(entries) == 0:
				return
			default:
				sum := FoldSessionSummary(nil, SessionKey{ProjectKey: projectKey, SessionID: e.SessionID}, entries)
				sum.MtimeMs = e.MtimeMs
				var ok bool
				if info, ok = SummaryToSessionInfo(sum, projectPath); !ok {
					return
				}
			}
			mu.Lock()
			out[e.SessionID] = info
			mu.Unlock()
		}(e)
	}
	wg.Wait()
	return out
}

// GetSessionInfoFromStore reads the metadata of one session from a SessionStore with a
// single Load, whatever optional interfaces the store implements.
//
// It returns nil, nil when the session is not in the store, is a sidechain, or has nothing to
// show as a summary. A Load error is returned. The session's LastModified is the timestamp
// of its last entry, or the current time when that entry carries none, since a bare Load
// cannot ask the store for an mtime.
func GetSessionInfoFromStore(ctx context.Context, store SessionStore, projectKey, projectPath, sessionID string) (*SessionInfo, error) {
	key := SessionKey{ProjectKey: projectKey, SessionID: sessionID}
	entries, err := store.Load(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	sum := FoldSessionSummary(nil, key, entries)
	if ms, ok := isoToEpochMs(entries[len(entries)-1]["timestamp"]); ok {
		sum.MtimeMs = ms
	} else {
		sum.MtimeMs = time.Now().UnixMilli()
	}
	info, ok := SummaryToSessionInfo(sum, projectPath)
	if !ok {
		return nil, nil
	}
	return &info, nil
}
