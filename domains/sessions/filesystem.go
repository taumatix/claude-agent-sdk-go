package sessions

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FilesystemStore implements Store using JSONL files in ~/.claude/projects/<sanitized-path>/.
type FilesystemStore struct {
	baseDir string
}

// NewFilesystemStore creates a FilesystemStore rooted at the given base directory.
// If baseDir is empty, it defaults to ~/.claude/projects.
func NewFilesystemStore(baseDir string) (*FilesystemStore, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".claude", "projects")
	}
	return &FilesystemStore{baseDir: baseDir}, nil
}

// ListSessions returns all sessions stored under the project path directory.
func (s *FilesystemStore) ListSessions(projectPath string) ([]SessionInfo, error) {
	dir := s.projectDir(projectPath)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
		info, err := s.readSessionInfo(dir, sessionID, e)
		if err != nil {
			continue // skip corrupt sessions
		}
		sessions = append(sessions, info)
	}
	return sessions, nil
}

// ReadSession returns all messages from a session file.
func (s *FilesystemStore) ReadSession(sessionID string) ([]SessionMessage, error) {
	path, err := s.findSessionFile(sessionID)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	var messages []SessionMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg SessionMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip malformed lines
		}
		if msg.Type == "user" || msg.Type == "assistant" {
			messages = append(messages, msg)
		}
	}
	return messages, scanner.Err()
}

// WriteSessionMeta writes session metadata to a sidecar .meta.json file.
func (s *FilesystemStore) WriteSessionMeta(sessionID string, info SessionInfo) error {
	path, err := s.findSessionFile(sessionID)
	if err != nil {
		return err
	}
	metaPath := strings.TrimSuffix(path, ".jsonl") + ".meta.json"
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session meta: %w", err)
	}
	return os.WriteFile(metaPath, data, 0o644)
}

// DeleteSession removes the session .jsonl file and any .meta.json sidecar.
func (s *FilesystemStore) DeleteSession(sessionID string) error {
	path, err := s.findSessionFile(sessionID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session file: %w", err)
	}
	metaPath := strings.TrimSuffix(path, ".jsonl") + ".meta.json"
	_ = os.Remove(metaPath) // ignore error — sidecar may not exist
	return nil
}

// ForkSession creates a new session JSONL file containing messages up to upToMessage index.
func (s *FilesystemStore) ForkSession(sessionID string, upToMessage int) (string, error) {
	msgs, err := s.ReadSession(sessionID)
	if err != nil {
		return "", err
	}

	if upToMessage > len(msgs) {
		upToMessage = len(msgs)
	}

	srcPath, err := s.findSessionFile(sessionID)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(srcPath)

	newID := newUUID()
	newPath := filepath.Join(dir, newID+".jsonl")

	f, err := os.Create(newPath)
	if err != nil {
		return "", fmt.Errorf("create forked session file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, msg := range msgs[:upToMessage] {
		msg.SessionID = newID
		if err := enc.Encode(msg); err != nil {
			return "", fmt.Errorf("write forked message: %w", err)
		}
	}
	return newID, nil
}

// projectDir returns the directory for a given project path.
func (s *FilesystemStore) projectDir(projectPath string) string {
	if projectPath == "" {
		return s.baseDir
	}
	sanitized := sanitizePath(projectPath)
	return filepath.Join(s.baseDir, sanitized)
}

// findSessionFile searches for a session file by ID across all project subdirectories.
func (s *FilesystemStore) findSessionFile(sessionID string) (string, error) {
	// First check baseDir directly
	direct := filepath.Join(s.baseDir, sessionID+".jsonl")
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}

	// Search subdirectories
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("session %s not found", sessionID)
		}
		return "", fmt.Errorf("read base dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(s.baseDir, e.Name(), sessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("session %s not found", sessionID)
}

// readSessionInfo builds a SessionInfo from directory entry and optional sidecar.
func (s *FilesystemStore) readSessionInfo(dir, sessionID string, entry os.DirEntry) (SessionInfo, error) {
	info, err := entry.Info()
	if err != nil {
		return SessionInfo{}, err
	}

	si := SessionInfo{
		SessionID:    sessionID,
		Summary:      sessionID,
		LastModified: info.ModTime().UnixMilli(),
		FileSize:     info.Size(),
	}

	// Try reading sidecar
	metaPath := filepath.Join(dir, sessionID+".meta.json")
	if data, err := os.ReadFile(metaPath); err == nil {
		var meta SessionInfo
		if json.Unmarshal(data, &meta) == nil {
			if meta.Summary != "" {
				si.Summary = meta.Summary
			}
			si.CustomTitle = meta.CustomTitle
			si.FirstPrompt = meta.FirstPrompt
			si.GitBranch = meta.GitBranch
			si.Cwd = meta.Cwd
			si.Tag = meta.Tag
			si.CreatedAt = meta.CreatedAt
		}
	}

	return si, nil
}

// sanitizePath converts an absolute filesystem path to a safe directory name,
// matching the convention used by the Claude Code CLI.
func sanitizePath(path string) string {
	// Replace path separators and other unsafe characters with hyphens
	s := strings.ReplaceAll(path, string(os.PathSeparator), "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.TrimPrefix(s, "-")
	return s
}

// newUUID generates a random UUID v4 string.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
