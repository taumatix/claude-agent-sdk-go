package sessions

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// SessionSummaryEntry is an incrementally maintained summary of one session. A store gets
// it from FoldSessionSummary inside Append and persists it verbatim; Data is SDK-owned
// and a store must not interpret it. It survives a JSON round trip.
type SessionSummaryEntry struct {
	SessionID string `json:"session_id"`
	// MtimeMs is the storage write time of the summary in Unix epoch milliseconds. It must
	// come from the same clock as SessionListEntry.MtimeMs for the session, and the adapter
	// stamps it after persisting: FoldSessionSummary keeps what prev carried and never sets
	// it (a first summary has 0).
	MtimeMs int64          `json:"mtime"`
	Data    map[string]any `json:"data"`
}

// SessionSummaryLister is implemented by stores that keep SessionSummaryEntry sidecars, so
// that a listing is one call instead of a Load per session.
type SessionSummaryLister interface {
	// ListSessionSummaries returns the summaries of the project's sessions, in unspecified
	// order. Like ListSessions it leaves sub-transcripts out and returns an empty slice for
	// an unknown project.
	ListSessionSummaries(ctx context.Context, projectKey string) ([]SessionSummaryEntry, error)
}

var (
	commandNameRE = regexp.MustCompile(`<command-name>(.*?)</command-name>`)
	skipPromptRE  = regexp.MustCompile(`^(?:<local-command-stdout>|<session-start-hook>|<tick>|<goal>|` +
		`\[Request interrupted by user[^\]]*\]|` +
		`\s*<ide_opened_file>[\s\S]*</ide_opened_file>\s*$|` +
		`\s*<ide_selection>[\s\S]*</ide_selection>\s*$)`)
)

// lastWins maps a transcript entry key to the summary key it overwrites on every append.
var lastWins = [][2]string{
	{"customTitle", "custom_title"},
	{"aiTitle", "ai_title"},
	{"lastPrompt", "last_prompt"},
	{"summary", "summary_hint"},
	{"gitBranch", "git_branch"},
}

// FoldSessionSummary folds a batch of appended entries into the running summary of a
// session. prev is the previous summary of the same session, or nil for the first append;
// it is not modified. Do not call it for a key with a Subpath: a sub-transcript must not
// contribute to the main session's summary.
func FoldSessionSummary(prev *SessionSummaryEntry, key SessionKey, entries []SessionStoreEntry) SessionSummaryEntry {
	out := SessionSummaryEntry{SessionID: key.SessionID, Data: map[string]any{}}
	if prev != nil {
		out.SessionID, out.MtimeMs = prev.SessionID, prev.MtimeMs
		for k, v := range prev.Data {
			out.Data[k] = v
		}
	}
	data := out.Data
	for _, e := range entries {
		if _, ok := data["is_sidechain"]; !ok {
			data["is_sidechain"] = e["isSidechain"] == true
		}
		if _, ok := data["created_at"]; !ok {
			if ms, ok := isoToEpochMs(e["timestamp"]); ok {
				data["created_at"] = ms
			}
		}
		if _, ok := data["cwd"]; !ok {
			if cwd, _ := e["cwd"].(string); cwd != "" {
				data["cwd"] = cwd
			}
		}
		foldFirstPrompt(data, e)
		for _, f := range lastWins {
			if v, ok := e[f[0]].(string); ok {
				data[f[1]] = v
			}
		}
		if e["type"] == "tag" {
			if tag, _ := e["tag"].(string); tag != "" {
				data["tag"] = tag
			} else {
				delete(data, "tag")
			}
		}
	}
	return out
}

func isoToEpochMs(v any) (int64, bool) {
	s, ok := v.(string)
	if !ok {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, false
	}
	return t.UnixMilli(), true
}

func entryTextBlocks(e SessionStoreEntry) []string {
	msg, ok := e["message"].(map[string]any)
	if !ok {
		return nil
	}
	switch c := msg["content"].(type) {
	case string:
		return []string{c}
	case []any:
		var out []string
		for _, b := range c {
			if m, ok := b.(map[string]any); ok && m["type"] == "text" {
				if s, ok := m["text"].(string); ok {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

func foldFirstPrompt(data map[string]any, e SessionStoreEntry) {
	if locked, _ := data["first_prompt_locked"].(bool); locked || e["type"] != "user" {
		return
	}
	if e["isMeta"] == true || e["isCompactSummary"] == true {
		return
	}
	if msg, ok := e["message"].(map[string]any); ok {
		if blocks, ok := msg["content"].([]any); ok {
			for _, b := range blocks {
				if m, ok := b.(map[string]any); ok && m["type"] == "tool_result" {
					return
				}
			}
		}
	}
	for _, raw := range entryTextBlocks(e) {
		text := strings.TrimSpace(strings.ReplaceAll(raw, "\n", " "))
		if text == "" {
			continue
		}
		if m := commandNameRE.FindStringSubmatch(text); m != nil {
			if fb, _ := data["command_fallback"].(string); fb == "" {
				data["command_fallback"] = m[1]
			}
			continue
		}
		if skipPromptRE.MatchString(text) {
			continue
		}
		if utf8.RuneCountInString(text) > 200 {
			text = strings.TrimRight(string([]rune(text)[:200]), " \t\r\n\f\v") + "…"
		}
		data["first_prompt"], data["first_prompt_locked"] = text, true
		return
	}
}

// SummaryToSessionInfo converts a summary to a SessionInfo. It reports false for a
// sidechain session and for one with nothing to show as a summary. projectPath is the
// Cwd used when the transcript recorded none.
func SummaryToSessionInfo(e SessionSummaryEntry, projectPath string) (SessionInfo, bool) {
	str := func(k string) string { s, _ := e.Data[k].(string); return s }
	if side, _ := e.Data["is_sidechain"].(bool); side {
		return SessionInfo{}, false
	}
	first := str("command_fallback")
	if locked, _ := e.Data["first_prompt_locked"].(bool); locked {
		first = str("first_prompt")
	}
	title := str("custom_title")
	if title == "" {
		title = str("ai_title")
	}
	summary := title
	for _, s := range []string{str("last_prompt"), str("summary_hint"), first} {
		if summary == "" {
			summary = s
		}
	}
	if summary == "" {
		return SessionInfo{}, false
	}
	cwd := str("cwd")
	if cwd == "" {
		cwd = projectPath
	}
	return SessionInfo{
		SessionID:    e.SessionID,
		Summary:      summary,
		LastModified: e.MtimeMs,
		CustomTitle:  title,
		FirstPrompt:  first,
		GitBranch:    str("git_branch"),
		Cwd:          cwd,
		Tag:          str("tag"),
		CreatedAt:    int64Of(e.Data["created_at"]),
	}, true
}

func int64Of(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}
