package sessions

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// FileSessionStore is a SessionStore (with all three optional interfaces and summaries)
// that keeps transcripts as JSON-lines files under one directory, so they survive the
// process. It is a working adapter and a reference for writing one against a real backend:
// the mtime it reports for a session is the file system's, and the summary sidecar is
// stamped from that same clock after the append lands, which is what the listing's freshness
// rule needs.
//
// Layout: <dir>/<project>/<session>/main.jsonl, summary.json and sub/<subpath>.jsonl, every
// name hex-encoded so a key can never address a path outside dir. A key part therefore
// should stay under 120 bytes (hex doubles it against the 255-byte file-name limit). A store serialises its own
// writers; two processes appending to one directory are not coordinated.
type FileSessionStore struct {
	dir string
	mu  sync.Mutex
}

// NewFileSessionStore returns a store rooted at dir, which is created on first write.
func NewFileSessionStore(dir string) *FileSessionStore {
	return &FileSessionStore{dir: dir}
}

func encodePart(s string) string { return "x" + hex.EncodeToString([]byte(s)) }

func decodePart(name string) (string, bool) {
	if len(name) == 0 || name[0] != 'x' {
		return "", false
	}
	b, err := hex.DecodeString(name[1:])
	return string(b), err == nil
}

func (f *FileSessionStore) sessionDir(projectKey, sessionID string) string {
	return filepath.Join(f.dir, encodePart(projectKey), encodePart(sessionID))
}

func (f *FileSessionStore) transcriptPath(key SessionKey) string {
	d := f.sessionDir(key.ProjectKey, key.SessionID)
	if key.Subpath == "" {
		return filepath.Join(d, "main.jsonl")
	}
	return filepath.Join(d, "sub", encodePart(key.Subpath)+".jsonl")
}

// Append implements SessionStore.
func (f *FileSessionStore) Append(_ context.Context, key SessionKey, entries []SessionStoreEntry) error {
	if len(entries) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("sessions: encode entry: %w", err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.transcriptPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, werr := file.Write(buf.Bytes())
	if cerr := file.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	if key.Subpath != "" {
		return nil
	}
	return f.writeSummary(key, entries, path)
}

// writeSummary folds entries into the sidecar and stamps it with the transcript's mtime as
// the file system now reports it, the clock ListSessions reads.
func (f *FileSessionStore) writeSummary(key SessionKey, entries []SessionStoreEntry, transcript string) error {
	sumPath := filepath.Join(f.sessionDir(key.ProjectKey, key.SessionID), "summary.json")
	var prev *SessionSummaryEntry
	if raw, err := os.ReadFile(sumPath); err == nil {
		var p SessionSummaryEntry
		if json.Unmarshal(raw, &p) == nil {
			prev = &p
		}
	}
	st, err := os.Stat(transcript)
	if err != nil {
		return err
	}
	sum := FoldSessionSummary(prev, key, entries)
	sum.MtimeMs = st.ModTime().UnixMilli()
	raw, err := json.Marshal(sum)
	if err != nil {
		return err
	}
	tmp := sumPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, sumPath)
}

// Load implements SessionStore.
func (f *FileSessionStore) Load(_ context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := os.ReadFile(f.transcriptPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []SessionStoreEntry
	for i, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var e SessionStoreEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("sessions: %s line %d: %w", f.transcriptPath(key), i+1, err)
		}
		out = append(out, e)
	}
	return out, nil
}

// ListSessions implements SessionLister.
func (f *FileSessionStore) ListSessions(_ context.Context, projectKey string) ([]SessionListEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []SessionListEntry{}
	for _, id := range f.sessionIDs(projectKey) {
		st, err := os.Stat(filepath.Join(f.sessionDir(projectKey, id), "main.jsonl"))
		if err != nil {
			continue
		}
		out = append(out, SessionListEntry{SessionID: id, MtimeMs: st.ModTime().UnixMilli()})
	}
	return out, nil
}

func (f *FileSessionStore) sessionIDs(projectKey string) []string {
	dirs, err := os.ReadDir(filepath.Join(f.dir, encodePart(projectKey)))
	if err != nil {
		return nil
	}
	var ids []string
	for _, d := range dirs {
		if id, ok := decodePart(d.Name()); ok && d.IsDir() {
			ids = append(ids, id)
		}
	}
	return ids
}

// Delete implements SessionDeleter.
func (f *FileSessionStore) Delete(_ context.Context, key SessionKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key.Subpath != "" {
		err := os.Remove(f.transcriptPath(key))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.RemoveAll(f.sessionDir(key.ProjectKey, key.SessionID))
}

// ListSubkeys implements SubkeyLister.
func (f *FileSessionStore) ListSubkeys(_ context.Context, key SessionKey) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	files, err := os.ReadDir(filepath.Join(f.sessionDir(key.ProjectKey, key.SessionID), "sub"))
	if err != nil {
		return out, nil
	}
	for _, e := range files {
		name := e.Name()
		if filepath.Ext(name) != ".jsonl" {
			continue
		}
		if sub, ok := decodePart(name[:len(name)-len(".jsonl")]); ok {
			out = append(out, sub)
		}
	}
	sort.Strings(out)
	return out, nil
}

// ListSessionSummaries implements SessionSummaryLister. A session whose summary file is
// missing or unreadable is left out, which the listing gap-fills from Load.
func (f *FileSessionStore) ListSessionSummaries(_ context.Context, projectKey string) ([]SessionSummaryEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []SessionSummaryEntry{}
	for _, id := range f.sessionIDs(projectKey) {
		raw, err := os.ReadFile(filepath.Join(f.sessionDir(projectKey, id), "summary.json"))
		if err != nil {
			continue
		}
		var s SessionSummaryEntry
		if json.Unmarshal(raw, &s) != nil || s.SessionID != id {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}
