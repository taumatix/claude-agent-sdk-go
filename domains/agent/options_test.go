package agent_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

func testBuildCLIArgsContains(t *testing.T, opts agent.Options, flag string) {
	t.Helper()
	args := agent.BuildCLIArgs(opts)
	assert.Contains(t, args, flag)
}

func testBuildCLIArgsNotContains(t *testing.T, opts agent.Options, flag string) {
	t.Helper()
	args := agent.BuildCLIArgs(opts)
	assert.NotContains(t, args, flag)
}

func TestBuildCLIArgs_SettingSources(t *testing.T) {
	// nil: flag omitted
	testBuildCLIArgsNotContains(t, agent.Options{SettingSources: nil}, "--setting-sources")

	// empty slice: flag omitted (matches Python fix)
	testBuildCLIArgsNotContains(t, agent.Options{SettingSources: []string{}}, "--setting-sources")

	// non-empty: flag present
	testBuildCLIArgsContains(t, agent.Options{SettingSources: []string{"global", "local"}}, "--setting-sources")
}

func TestBuildCLIArgs_SessionAndPluginFlags(t *testing.T) {
	assert.NotContains(t, agent.BuildCLIArgs(agent.Options{}), "--json-schema")

	args := agent.BuildCLIArgs(agent.Options{
		JSONSchema:           `{"type":"object"}`,
		NoSessionPersistence: true,
		StrictMCPConfig:      true,
		PluginDirs:           []string{"/p/a", "/p/b.zip"},
		IncludeHookEvents:    true,
	})
	assert.Contains(t, args, "--no-session-persistence")
	assert.Contains(t, args, "--strict-mcp-config")
	assert.Contains(t, args, "--include-hook-events")
	i := indexOf(args, "--json-schema")
	assert.GreaterOrEqual(t, i, 0)
	assert.Equal(t, `{"type":"object"}`, args[i+1])
	var dirs []string
	for j, a := range args {
		if a == "--plugin-dir" {
			dirs = append(dirs, args[j+1])
		}
	}
	assert.Equal(t, []string{"/p/a", "/p/b.zip"}, dirs, "--plugin-dir repeats, which ExtraArgs cannot express")
}

func indexOf(args []string, flag string) int {
	for i, a := range args {
		if a == flag {
			return i
		}
	}
	return -1
}

func TestBuildCLIArgs_AgentsSlashCommandsAndPluginURLs(t *testing.T) {
	assert.NotContains(t, agent.BuildCLIArgs(agent.Options{}), "--agents")

	args := agent.BuildCLIArgs(agent.Options{
		Agents: map[string]agent.AgentDefinition{
			"reviewer": {Description: "Reviews code", Prompt: "You are a code reviewer"},
			"a":        {Description: "d", Prompt: "p"},
		},
		DisableSlashCommands: true,
		PluginURLs:           []string{"https://x/a.zip", "https://x/b.zip"},
	})
	i := indexOf(args, "--agents")
	assert.GreaterOrEqual(t, i, 0)
	assert.JSONEq(t,
		`{"a":{"description":"d","prompt":"p"},"reviewer":{"description":"Reviews code","prompt":"You are a code reviewer"}}`,
		args[i+1])
	assert.Contains(t, args, "--disable-slash-commands")
	assert.Equal(t, 2, strings.Count(strings.Join(args, " "), "--plugin-url"))
}

func TestBuildCLIArgs_WorktreePRAndPermissionPromptFlags(t *testing.T) {
	none := agent.BuildCLIArgs(agent.Options{WorktreeName: "x"})
	for _, f := range []string{"--worktree", "--from-pr", "--permission-prompt-tool", "--permission-prompts"} {
		assert.NotContains(t, none, f)
	}

	args := agent.BuildCLIArgs(agent.Options{
		Worktree: true, WorktreeName: "fix-a", FromPR: "123",
		PermissionPromptTool: "mcp__auth__approve", PermissionPrompts: "none",
	})
	i := indexOf(args, "--worktree")
	assert.GreaterOrEqual(t, i, 0)
	assert.Equal(t, "fix-a", args[i+1])
	assert.Equal(t, "123", args[indexOf(args, "--from-pr")+1])
	assert.Equal(t, "mcp__auth__approve", args[indexOf(args, "--permission-prompt-tool")+1])
	assert.Equal(t, "none", args[indexOf(args, "--permission-prompts")+1])

	bare := agent.BuildCLIArgs(agent.Options{Worktree: true})
	i = indexOf(bare, "--worktree")
	assert.GreaterOrEqual(t, i, 0)
	assert.True(t, strings.HasPrefix(bare[i+1], "--"), "no name is sent for an unnamed worktree")
}
