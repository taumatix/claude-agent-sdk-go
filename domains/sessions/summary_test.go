package sessions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sk = SessionKey{ProjectKey: "p", SessionID: "s"}

func user(text string, extra ...any) SessionStoreEntry {
	e := SessionStoreEntry{"type": "user", "message": map[string]any{"content": text}}
	for i := 0; i+1 < len(extra); i += 2 {
		e[extra[i].(string)] = extra[i+1]
	}
	return e
}

func TestFoldFirstPrompt(t *testing.T) {
	long := strings.Repeat("é", 250)
	cases := []struct {
		name    string
		entries []SessionStoreEntry
		want    string
	}{
		{"plain", []SessionStoreEntry{user("hello\nworld")}, "hello world"},
		{"skips meta and tool results", []SessionStoreEntry{
			user("meta", "isMeta", true),
			{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result"}}}},
			user("real")}, "real"},
		{"skips generated prompts", []SessionStoreEntry{user("<tick>"), user("[Request interrupted by user for tool use]"),
			user("<ide_opened_file>x</ide_opened_file>"), user("asked")}, "asked"},
		{"command only falls back to its name", []SessionStoreEntry{user("<command-name>/init</command-name> go")}, "/init"},
		{"real prompt beats a command", []SessionStoreEntry{user("<command-name>/init</command-name>"), user("real")}, "real"},
		{"truncates by characters", []SessionStoreEntry{user(long)}, strings.Repeat("é", 200) + "…"},
		{"text blocks", []SessionStoreEntry{{"type": "user", "message": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "  block  "}}}}}, "block"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info, ok := SummaryToSessionInfo(FoldSessionSummary(nil, sk, c.entries), "")
			require.True(t, ok)
			assert.Equal(t, c.want, info.FirstPrompt)
		})
	}
}

func TestFoldLatchesAndOverwrites(t *testing.T) {
	a := FoldSessionSummary(nil, sk, []SessionStoreEntry{
		{"type": "x", "timestamp": "2024-01-01T00:00:00.500Z", "cwd": "/one", "customTitle": "a", "gitBranch": "main"},
		{"type": "tag", "tag": "t1"},
	})
	b := FoldSessionSummary(&a, sk, []SessionStoreEntry{
		{"type": "x", "timestamp": "2025-01-01T00:00:00Z", "cwd": "/two", "customTitle": "b", "summary": "hint"},
	})
	info, ok := SummaryToSessionInfo(b, "/proj")
	require.True(t, ok)
	assert.Equal(t, "/one", info.Cwd, "cwd is set once")
	assert.Equal(t, int64(1704067200500), info.CreatedAt, "created_at latches the first timestamp")
	assert.Equal(t, "b", info.CustomTitle, "title is last-wins")
	assert.Equal(t, "main", info.GitBranch)
	assert.Equal(t, "t1", info.Tag)
	assert.Equal(t, "a", a.Data["custom_title"], "prev is not modified")

	c := FoldSessionSummary(&b, sk, []SessionStoreEntry{{"type": "tag", "tag": ""}})
	_, has := c.Data["tag"]
	assert.False(t, has, "an empty tag clears it")
}

func TestSummaryToSessionInfoFilters(t *testing.T) {
	_, ok := SummaryToSessionInfo(FoldSessionSummary(nil, sk, []SessionStoreEntry{{"type": "x", "isSidechain": true, "customTitle": "t"}}), "")
	assert.False(t, ok, "sidechains are left out")
	_, ok = SummaryToSessionInfo(FoldSessionSummary(nil, sk, []SessionStoreEntry{{"type": "x"}}), "")
	assert.False(t, ok, "nothing to show")
	info, ok := SummaryToSessionInfo(FoldSessionSummary(nil, sk, []SessionStoreEntry{
		{"type": "x", "lastPrompt": "lp", "summary": "hint"}}), "/proj")
	require.True(t, ok)
	assert.Equal(t, "lp", info.Summary, "last prompt outranks the hint")
	assert.Equal(t, "/proj", info.Cwd, "project path fills a missing cwd")
}

// A summary persisted by a JSON-backed store comes back with float64 numbers and must
// still fold and convert.
func TestSummarySurvivesJSON(t *testing.T) {
	m := NewMemorySessionStore()
	ctx := context.Background()
	require.NoError(t, m.Append(ctx, sk, []SessionStoreEntry{user("hi", "timestamp", "2024-01-01T00:00:00Z")}))
	got, err := m.ListSessionSummaries(ctx, "p")
	require.NoError(t, err)
	raw, err := json.Marshal(got[0])
	require.NoError(t, err)
	var back SessionSummaryEntry
	require.NoError(t, json.Unmarshal(raw, &back))
	next := FoldSessionSummary(&back, sk, []SessionStoreEntry{{"type": "x", "customTitle": "t"}})
	info, ok := SummaryToSessionInfo(next, "")
	require.True(t, ok)
	assert.Equal(t, "t", info.Summary)
	assert.Equal(t, int64(1704067200000), info.CreatedAt)
	assert.Equal(t, "hi", info.FirstPrompt)
}
