package agent

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/taumatix/claude-agent-sdk-go/domains/sessions"
)

func TestSessionKeyForPath(t *testing.T) {
	root := filepath.FromSlash("/cfg/projects")
	p := func(s string) string { return filepath.Join(root, filepath.FromSlash(s)) }

	cases := []struct {
		name string
		path string
		want sessions.SessionKey
		ok   bool
	}{
		{"main", p("proj/s1.jsonl"), sessions.SessionKey{ProjectKey: "proj", SessionID: "s1"}, true},
		{"subagent", p("proj/s1/subagents/agent-2.jsonl"), sessions.SessionKey{ProjectKey: "proj", SessionID: "s1", Subpath: "subagents/agent-2"}, true},
		{"nested", p("proj/s1/subagents/w/agent-2.jsonl"), sessions.SessionKey{ProjectKey: "proj", SessionID: "s1", Subpath: "subagents/w/agent-2"}, true},
		{"outside", "/elsewhere/proj/s1.jsonl", sessions.SessionKey{}, false},
		{"parent escape", filepath.FromSlash("/cfg/other/s1.jsonl"), sessions.SessionKey{}, false},
		{"no project", p("s1.jsonl"), sessions.SessionKey{}, false},
		{"not jsonl", p("proj/s1.txt"), sessions.SessionKey{}, false},
		{"three parts", p("proj/s1/x.jsonl"), sessions.SessionKey{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := sessionKeyForPath(c.path, root)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.want, got)
		})
	}
}
