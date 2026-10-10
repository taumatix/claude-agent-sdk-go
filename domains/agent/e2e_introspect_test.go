package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

// A caller could not ask a running session which MCP servers had failed to
// start or how full the context window was; the only answer was the model's.
// These drive the stub CLI over real pipes, which answers with the bodies the
// CLI sends.

func connectedClient(t *testing.T) (*agent.Client, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	client := agent.NewClient(fakeOpts("old-cli", nil))
	require.NoError(t, client.Connect(ctx))
	t.Cleanup(func() { _ = client.Disconnect() })
	return client, ctx
}

func TestE2E_MCPStatusReportsEachServer(t *testing.T) {
	client, ctx := connectedClient(t)

	st, err := client.MCPStatus(ctx)
	require.NoError(t, err)
	require.Len(t, st.MCPServers, 2)

	files := st.MCPServers[0]
	assert.Equal(t, "files", files.Name)
	assert.Equal(t, "connected", files.Status)
	require.NotNil(t, files.ServerInfo)
	assert.Equal(t, "1.2.0", files.ServerInfo.Version)
	assert.Equal(t, "project", files.Scope)
	require.Len(t, files.Tools, 1)
	assert.Equal(t, "read", files.Tools[0].Name)
	assert.JSONEq(t, `{"type":"stdio","command":"files-srv"}`, string(files.Config))

	failed := st.MCPServers[1]
	assert.Equal(t, "failed", failed.Status)
	assert.Equal(t, "spawn ENOENT", failed.Error)
	assert.Nil(t, failed.ServerInfo)
}

func TestE2E_ContextUsageReportsTheWindow(t *testing.T) {
	client, ctx := connectedClient(t)

	u, err := client.ContextUsage(ctx)
	require.NoError(t, err)
	assert.Equal(t, 4100, u.TotalTokens)
	assert.Equal(t, 200000, u.MaxTokens)
	assert.InDelta(t, 2.05, u.Percentage, 0.001)
	assert.Equal(t, "claude-test", u.Model)
	assert.True(t, u.IsAutoCompactEnabled)
	assert.Equal(t, 167000, u.AutoCompactThreshold)
	require.Len(t, u.Categories, 2)
	assert.Equal(t, "System prompt", u.Categories[0].Name)
	assert.Equal(t, 3200, u.Categories[0].Tokens)
	assert.JSONEq(t, `[{"path":"CLAUDE.md","type":"project","tokens":120}]`, string(u.MemoryFiles))
}

func statusOf(t *testing.T, client *agent.Client, ctx context.Context, name string) string {
	t.Helper()
	st, err := client.MCPStatus(ctx)
	require.NoError(t, err)
	for _, s := range st.MCPServers {
		if s.Name == name {
			return s.Status
		}
	}
	t.Fatalf("no server %q in the status", name)
	return ""
}

// Seeing a server "failed" is only useful if the caller can then retry it. The
// stub keeps state, so the status read afterwards shows what the CLI was told.
func TestE2E_AFailedMCPServerCanBeReconnected(t *testing.T) {
	client, ctx := connectedClient(t)
	require.Equal(t, "failed", statusOf(t, client, ctx, "search"))

	require.NoError(t, client.ReconnectMCPServer(ctx, "search"))
	assert.Equal(t, "connected", statusOf(t, client, ctx, "search"))
}

func TestE2E_AnMCPServerCanBeDisabledAndEnabled(t *testing.T) {
	client, ctx := connectedClient(t)

	require.NoError(t, client.ToggleMCPServer(ctx, "files", false))
	assert.Equal(t, "disabled", statusOf(t, client, ctx, "files"))
	assert.Equal(t, "failed", statusOf(t, client, ctx, "search"), "toggling one server touched another")

	require.NoError(t, client.ToggleMCPServer(ctx, "files", true))
	assert.Equal(t, "connected", statusOf(t, client, ctx, "files"))
}

func TestE2E_AnUnknownMCPServerIsTheCLIsError(t *testing.T) {
	client, ctx := connectedClient(t)

	err := client.ReconnectMCPServer(ctx, "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no MCP server named nope")
	err = client.ToggleMCPServer(ctx, "nope", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no MCP server named nope")
}

func TestE2E_IntrospectionNeedsAConnection(t *testing.T) {
	client := agent.NewClient(fakeOpts("old-cli", nil))
	_, err := client.MCPStatus(context.Background())
	assert.Error(t, err)
	_, err = client.ContextUsage(context.Background())
	assert.Error(t, err)
	assert.Error(t, client.ReconnectMCPServer(context.Background(), "files"))
	assert.Error(t, client.ToggleMCPServer(context.Background(), "files", true))
}
