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
