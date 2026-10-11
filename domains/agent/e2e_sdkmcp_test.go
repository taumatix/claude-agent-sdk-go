package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

// An SDK MCP server runs in the caller's process. The stub CLI plays the CLI's
// MCP client: it sends JSON-RPC over mcp_message control requests on the real
// pipes and reports each answer it read back. Not covered: a live CLI (the
// request and response shapes are from upstream's source, not captured).

func calcServer(calls *[]string) *agent.SDKMCPServer {
	return agent.NewSDKMCPServer("calc", "1.2.3",
		agent.MCPTool{
			Name:        "add",
			Description: "Adds two numbers",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}}}`),
			Handler: func(_ context.Context, args json.RawMessage) (agent.MCPToolResult, error) {
				var in struct{ A, B float64 }
				if err := json.Unmarshal(args, &in); err != nil {
					return agent.MCPToolResult{}, err
				}
				*calls = append(*calls, string(args))
				return agent.MCPToolResult{Content: []agent.MCPContent{agent.TextContent("5")}}, nil
			},
		},
		agent.MCPTool{
			Name: "fail",
			Handler: func(context.Context, json.RawMessage) (agent.MCPToolResult, error) {
				return agent.MCPToolResult{}, errors.New("disk full")
			},
		},
		agent.MCPTool{
			Name: "explode",
			Handler: func(context.Context, json.RawMessage) (agent.MCPToolResult, error) {
				panic("boom")
			},
		},
	)
}

func TestE2E_SDKMCPServerAnswersTheCLIsMCPClient(t *testing.T) {
	var calls []string
	opts := fakeOpts("old-cli-sdk-mcp", nil)
	opts.MCPServers = []agent.MCPServerConfig{
		agent.SDKMCPServerConfig(calcServer(&calls)),
		{Name: "files", Type: "stdio", Command: "files-mcp"},
	}

	var report struct {
		Answers   []json.RawMessage `json:"answers"`
		MCPConfig string            `json:"mcp_config"`
	}
	require.NoError(t, json.Unmarshal([]byte(reportedAnswer(t, opts)), &report))
	require.Len(t, report.Answers, 9)

	rpc := func(i int) map[string]any {
		var body struct {
			MCPResponse map[string]any `json:"mcp_response"`
		}
		require.NoError(t, json.Unmarshal(report.Answers[i], &body), string(report.Answers[i]))
		return body.MCPResponse
	}

	assert.Equal(t, map[string]any{
		"jsonrpc": "2.0", "id": float64(1),
		"result": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "calc", "version": "1.2.3"},
		},
	}, rpc(0))
	assert.Equal(t, map[string]any{"jsonrpc": "2.0", "result": map[string]any{}}, rpc(1),
		"a notification is acknowledged without a JSON-RPC reply")

	tools := rpc(2)["result"].(map[string]any)["tools"].([]any)
	require.Len(t, tools, 3)
	add := tools[0].(map[string]any)
	assert.Equal(t, "add", add["name"])
	assert.Equal(t, "Adds two numbers", add["description"])
	assert.Contains(t, add["inputSchema"], "properties")
	assert.Equal(t, map[string]any{"type": "object"}, tools[1].(map[string]any)["inputSchema"],
		"a tool with no schema takes any object")

	assert.Equal(t, map[string]any{
		"jsonrpc": "2.0", "id": float64(3),
		"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "5"}}},
	}, rpc(3))
	assert.JSONEq(t, `{"a":2,"b":3}`, calls[0])

	failed := rpc(4)["result"].(map[string]any)
	assert.Equal(t, true, failed["isError"])
	assert.Equal(t, "disk full", failed["content"].([]any)[0].(map[string]any)["text"])
	panicked := rpc(5)["result"].(map[string]any)
	assert.Equal(t, true, panicked["isError"], "a panicking tool fails its own call")
	assert.Contains(t, panicked["content"].([]any)[0].(map[string]any)["text"], "boom")
	unknown := rpc(6)["result"].(map[string]any)
	assert.Equal(t, true, unknown["isError"])
	assert.Equal(t, "Unknown tool: nope", unknown["content"].([]any)[0].(map[string]any)["text"])

	assert.Equal(t, float64(-32601), rpc(7)["error"].(map[string]any)["code"])
	missing := rpc(8)
	assert.Equal(t, float64(8), missing["id"])
	assert.Equal(t, "Server 'missing' not found", missing["error"].(map[string]any)["message"])

	// The CLI is told the server exists and nothing more; the other server is
	// configured as before.
	var cfg struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(report.MCPConfig), &cfg), report.MCPConfig)
	assert.Equal(t, map[string]any{"type": "sdk", "name": "calc"}, cfg.MCPServers["calc"])
	assert.Equal(t, "files-mcp", cfg.MCPServers["files"]["command"])
}

func TestE2E_TwoSDKMCPServersWithOneNameAreRefused(t *testing.T) {
	var calls []string
	opts := fakeOpts("old-cli-sdk-mcp", nil)
	opts.MCPServers = []agent.MCPServerConfig{
		agent.SDKMCPServerConfig(calcServer(&calls)),
		agent.SDKMCPServerConfig(calcServer(&calls)),
	}
	for _, err := range agent.Query(context.Background(), "x", opts) {
		require.ErrorContains(t, err, "two SDK MCP servers are named calc")
		return
	}
	t.Fatal("Query yielded nothing")
}

// A typed tool is served like any other: the stub CLI lists it, sees the derived schema, and a
// call decodes into the Go struct. Not covered: a live CLI.
func TestE2E_ATypedMCPToolIsListedWithItsSchemaAndCalled(t *testing.T) {
	type addArgs struct {
		A float64 `json:"a" description:"first addend"`
		B float64 `json:"b"`
	}
	var got addArgs
	srv := agent.NewSDKMCPServer("calc", "1.2.3",
		agent.NewTypedMCPTool("add", "Adds two numbers", func(_ context.Context, in addArgs) (agent.MCPToolResult, error) {
			got = in
			return agent.MCPToolResult{Content: []agent.MCPContent{agent.TextContent("5")}}, nil
		}),
		agent.MCPTool{Name: "fail", Handler: func(context.Context, json.RawMessage) (agent.MCPToolResult, error) {
			return agent.MCPToolResult{}, errors.New("disk full")
		}},
		agent.MCPTool{Name: "explode", Handler: func(context.Context, json.RawMessage) (agent.MCPToolResult, error) {
			panic("boom")
		}},
	)
	opts := fakeOpts("old-cli-sdk-mcp", nil)
	opts.MCPServers = []agent.MCPServerConfig{agent.SDKMCPServerConfig(srv)}

	var report struct {
		Answers []json.RawMessage `json:"answers"`
	}
	require.NoError(t, json.Unmarshal([]byte(reportedAnswer(t, opts)), &report))
	require.Len(t, report.Answers, 9)

	var list struct {
		MCPResponse struct {
			Result struct {
				Tools []struct {
					Name        string          `json:"name"`
					InputSchema json.RawMessage `json:"inputSchema"`
				} `json:"tools"`
			} `json:"result"`
		} `json:"mcp_response"`
	}
	require.NoError(t, json.Unmarshal(report.Answers[2], &list))
	require.Equal(t, "add", list.MCPResponse.Result.Tools[0].Name)
	assert.JSONEq(t, `{"type":"object","required":["a","b"],"properties":{"a":{"type":"number","description":"first addend"},"b":{"type":"number"}}}`,
		string(list.MCPResponse.Result.Tools[0].InputSchema))
	assert.Equal(t, addArgs{A: 2, B: 3}, got, "the call reached the typed handler decoded")
}
