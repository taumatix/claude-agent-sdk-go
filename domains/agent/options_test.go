package agent_test

import (
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
