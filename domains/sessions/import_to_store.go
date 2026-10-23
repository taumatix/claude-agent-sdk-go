package sessions

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ImportOptions refine ImportSessionToStore. The zero value reads
// ~/.claude/projects (or $CLAUDE_CONFIG_DIR/projects), searches every project
// directory, imports subagent transcripts and appends 500 entries at a time.
type ImportOptions struct {
	// ProjectsDir is the CLI's projects directory. Empty uses $CLAUDE_CONFIG_DIR/projects,
	// then ~/.claude/projects.
	ProjectsDir string
	// Directory is the project directory the session belongs to (the same path
	// FilesystemStore.ListSessions takes). Empty searches every project directory.
	// Unlike upstream, git worktrees of Directory are not tried.
	Directory string
	// SkipSubagents leaves out <sessionId>/subagents/** and its .meta.json sidecars.
	SkipSubagents bool
	// BatchSize is the most entries per Append; zero or less means 500. A batch also
	// ends at 1 MiB of line text.
	BatchSize int
}

const (
	importMaxEntries = 500
	importMaxBytes   = 1 << 20
)

// ErrSessionFileNotFound is returned when ImportSessionToStore finds no non-empty
// transcript for the session on disk.
var ErrSessionFileNotFound = errors.New("sessions: session transcript not found on disk")

// ImportSessionToStore replays a session the CLI wrote under its projects directory into
// store, the counterpart of upstream's import_session_to_store: for migrating local
// sessions to a remote store, or catching a store up after a mirror_error. The store's
// project key is the name of the on-disk project directory the file was found in, so an
// imported session is keyed like a live-mirrored one. Subagent transcripts are keyed
// "subagents/<...>" and each .meta.json sidecar becomes an agent_metadata entry appended
// after its transcript. Appends are in file order; a store should treat an entry's uuid as
// an idempotency key, which makes re-importing duplicate-safe.
//
// A line that is not a JSON object is an error and stops the import after the batches
// already appended. Written from upstream's source; it has not been run against a
// projects directory a live CLI filled.
func ImportSessionToStore(ctx context.Context, sessionID string, store SessionStore, opts ImportOptions) error {
	if !sessionIDRE.MatchString(sessionID) {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	projects := opts.ProjectsDir
	if projects == "" {
		if cfg := os.Getenv("CLAUDE_CONFIG_DIR"); cfg != "" {
			projects = filepath.Join(cfg, "projects")
		} else if home, err := os.UserHomeDir(); err == nil {
			projects = filepath.Join(home, ".claude", "projects")
		} else {
			return fmt.Errorf("sessions: find projects directory: %w", err)
		}
	}
	file := findSessionTranscript(projects, sessionID, opts.Directory)
	if file == "" {
		return fmt.Errorf("%w: %s", ErrSessionFileNotFound, sessionID)
	}
	batch := opts.BatchSize
	if batch <= 0 {
		batch = importMaxEntries
	}
	projectKey := filepath.Base(filepath.Dir(file))
	if err := appendJSONLInBatches(ctx, file, SessionKey{ProjectKey: projectKey, SessionID: sessionID}, store, batch); err != nil {
		return err
	}
	if opts.SkipSubagents {
		return nil
	}

	sessionDir := strings.TrimSuffix(file, ".jsonl")
	for _, sub := range collectJSONLFiles(filepath.Join(sessionDir, "subagents")) {
		rel, err := filepath.Rel(sessionDir, sub)
		if err != nil {
			return err
		}
		key := SessionKey{
			ProjectKey: projectKey,
			SessionID:  sessionID,
			Subpath:    strings.TrimSuffix(filepath.ToSlash(rel), ".jsonl"),
		}
		if err := appendJSONLInBatches(ctx, sub, key, store, batch); err != nil {
			return err
		}
		// The transcript does not contain agent_metadata; the CLI keeps it in a sidecar.
		// A missing, corrupt or non-object sidecar counts as absent.
		raw, err := os.ReadFile(strings.TrimSuffix(sub, ".jsonl") + ".meta.json")
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		var meta map[string]any
		if json.Unmarshal(raw, &meta) != nil || meta == nil {
			continue
		}
		meta["type"] = "agent_metadata" // last, so a stray "type" in the sidecar cannot shadow it
		if err := store.Append(ctx, key, []SessionStoreEntry{meta}); err != nil {
			return err
		}
	}
	return nil
}

func findSessionTranscript(projects, sessionID, directory string) string {
	name := sessionID + ".jsonl"
	nonEmpty := func(dir string) string {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
			return p
		}
		return ""
	}
	if directory != "" {
		fs := &FilesystemStore{baseDir: projects}
		return nonEmpty(fs.projectDir(directory))
	}
	entries, err := os.ReadDir(projects)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			if p := nonEmpty(filepath.Join(projects, e.Name())); p != "" {
				return p
			}
		}
	}
	return ""
}

func appendJSONLInBatches(ctx context.Context, path string, key SessionKey, store SessionStore, size int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var batch []SessionStoreEntry
	nbytes := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := store.Append(ctx, key, batch)
		batch, nbytes = nil, 0
		return err
	}
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(line) > 0 {
				var e SessionStoreEntry
				if jerr := json.Unmarshal(line, &e); jerr != nil || e == nil {
					return fmt.Errorf("sessions: %s: line is not a JSON object", path)
				}
				batch = append(batch, e)
				nbytes += len(line)
				if len(batch) >= size || nbytes >= importMaxBytes {
					if ferr := flush(); ferr != nil {
						return ferr
					}
				}
			}
		}
		if err == io.EOF {
			return flush()
		}
		if err != nil {
			return err
		}
	}
}

// collectJSONLFiles returns every *.jsonl under dir, sorted per directory, none if dir is absent.
func collectJSONLFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []string
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		switch {
		case e.IsDir():
			out = append(out, collectJSONLFiles(p)...)
		case strings.HasSuffix(e.Name(), ".jsonl"):
			out = append(out, p)
		}
	}
	return out
}
