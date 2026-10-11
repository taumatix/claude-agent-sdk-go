package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/taumatix/claude-agent-sdk-go/domains/sessions"
	"github.com/taumatix/claude-agent-sdk-go/shared/reqid"
)

// SessionStoreFlush chooses when mirrored transcript entries are appended to
// Options.SessionStore.
type SessionStoreFlush string

const (
	// SessionStoreFlushBatched holds entries until a result arrives, the buffer
	// passes mirrorMaxEntries or mirrorMaxBytes, or the session ends. This is the
	// default.
	SessionStoreFlushBatched SessionStoreFlush = "batched"
	// SessionStoreFlushEager appends each frame as it arrives.
	SessionStoreFlushEager SessionStoreFlush = "eager"
)

const (
	mirrorMaxEntries  = 500
	mirrorMaxBytes    = 1 << 20
	mirrorAppendLimit = 60 * time.Second
	mirrorAttempts    = 3
)

var mirrorBackoff = [mirrorAttempts - 1]time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

type mirrorItem struct {
	path    string
	entries []sessions.SessionStoreEntry
}

// transcriptMirror buffers transcript_mirror frames and appends them to a store.
// It is used from the read loop only, so a flush holds the loop for as long as the
// store takes: that back-pressure is deliberate, and keeps appends in order and
// every error report ahead of the result that follows it.
type transcriptMirror struct {
	store       sessions.SessionStore
	projectsDir string
	eager       bool
	pending     []mirrorItem
	entries     int
	bytes       int
	report      func(key *sessions.SessionKey, err string)
}

func newTranscriptMirror(opts Options, report func(*sessions.SessionKey, string)) *transcriptMirror {
	if opts.SessionStore == nil {
		return nil
	}
	return &transcriptMirror{
		store:       opts.SessionStore,
		projectsDir: filepath.Join(claudeConfigDir(opts.Env), "projects"),
		eager:       opts.SessionStoreFlush == SessionStoreFlushEager,
		report:      report,
	}
}

func claudeConfigDir(env map[string]string) string {
	if d := env["CLAUDE_CONFIG_DIR"]; d != "" {
		return d
	}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

func (m *transcriptMirror) enqueue(ctx context.Context, path string, entries []map[string]any) {
	if len(entries) == 0 {
		return
	}
	conv := make([]sessions.SessionStoreEntry, len(entries))
	for i, e := range entries {
		conv[i] = e
	}
	size := 0
	if raw, err := json.Marshal(entries); err == nil {
		size = len(raw)
	}
	m.pending = append(m.pending, mirrorItem{path: path, entries: conv})
	m.entries += len(conv)
	m.bytes += size
	if m.eager || m.entries > mirrorMaxEntries || m.bytes > mirrorMaxBytes {
		m.flush(ctx)
	}
}

// flush appends everything pending, one Append per file in first-seen order. It
// never fails: a batch that cannot be appended is dropped and reported.
func (m *transcriptMirror) flush(ctx context.Context) {
	items := m.pending
	m.pending, m.entries, m.bytes = nil, 0, 0

	var order []string
	byPath := map[string][]sessions.SessionStoreEntry{}
	for _, it := range items {
		if _, seen := byPath[it.path]; !seen {
			order = append(order, it.path)
		}
		byPath[it.path] = append(byPath[it.path], it.entries...)
	}
	for _, path := range order {
		key, ok := sessionKeyForPath(path, m.projectsDir)
		if !ok {
			m.report(nil, fmt.Sprintf("transcript path %q is not under %q; the CLI's CLAUDE_CONFIG_DIR probably differs from this process's", path, m.projectsDir))
			continue
		}
		if err := m.append(ctx, key, byPath[path]); err != nil {
			m.report(&key, err.Error())
		}
	}
}

func (m *transcriptMirror) append(ctx context.Context, key sessions.SessionKey, entries []sessions.SessionStoreEntry) error {
	var err error
	for attempt := 0; attempt < mirrorAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(mirrorBackoff[attempt-1]):
			case <-ctx.Done():
				return err
			}
		}
		actx, cancel := context.WithTimeout(ctx, mirrorAppendLimit)
		err = m.store.Append(actx, key, entries)
		timedOut := actx.Err() == context.DeadlineExceeded
		cancel()
		if err == nil {
			return nil
		}
		// A timed-out call may still land, so retrying it could duplicate.
		if timedOut {
			return err
		}
	}
	return err
}

// sessionKeyForPath derives the store key from a transcript path:
// <projects>/<project>/<session>.jsonl, or
// <projects>/<project>/<session>/<subpath>.jsonl for a sub-transcript.
func sessionKeyForPath(path, projectsDir string) (sessions.SessionKey, bool) {
	rel, err := filepath.Rel(projectsDir, path)
	if err != nil || filepath.IsAbs(rel) {
		return sessions.SessionKey{}, false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 || parts[0] == ".." {
		return sessions.SessionKey{}, false
	}
	if len(parts) == 2 && strings.HasSuffix(parts[1], ".jsonl") {
		return sessions.SessionKey{ProjectKey: parts[0], SessionID: strings.TrimSuffix(parts[1], ".jsonl")}, true
	}
	if len(parts) >= 4 {
		sub := parts[2:]
		sub[len(sub)-1] = strings.TrimSuffix(sub[len(sub)-1], ".jsonl")
		return sessions.SessionKey{ProjectKey: parts[0], SessionID: parts[1], Subpath: strings.Join(sub, "/")}, true
	}
	return sessions.SessionKey{}, false
}

// mirrorErrorLine is the wire form of the "mirror_error" system message.
func mirrorErrorLine(key *sessions.SessionKey, errText string) []byte {
	m := map[string]any{
		"type":       "system",
		"subtype":    "mirror_error",
		"error":      errText,
		"uuid":       reqid.New(),
		"session_id": "",
	}
	if key != nil {
		m["session_id"] = key.SessionID
		k := map[string]string{"project_key": key.ProjectKey, "session_id": key.SessionID}
		if key.Subpath != "" {
			k["subpath"] = key.Subpath
		}
		m["key"] = k
	}
	raw, _ := json.Marshal(m)
	return raw
}
