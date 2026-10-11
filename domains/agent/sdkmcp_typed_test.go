package agent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

type typedAddArgs struct {
	A     float64           `json:"a" description:"first addend"`
	B     float64           `json:"b"`
	Mode  string            `json:"mode,omitempty" enum:"fast,exact"`
	Tags  []string          `json:"tags,omitempty"`
	Limit *int              `json:"limit"`
	Meta  map[string]string `json:"meta,omitempty"`
	Skip  string            `json:"-"`
	note  string
}

func TestNewTypedMCPToolDerivesTheSchemaFromTheStruct(t *testing.T) {
	tool := agent.NewTypedMCPTool("add", "Adds", func(context.Context, typedAddArgs) (agent.MCPToolResult, error) {
		return agent.MCPToolResult{}, nil
	})
	assert.JSONEq(t, `{
		"type":"object",
		"required":["a","b"],
		"properties":{
			"a":{"type":"number","description":"first addend"},
			"b":{"type":"number"},
			"mode":{"type":"string","enum":["fast","exact"]},
			"tags":{"type":"array","items":{"type":"string"}},
			"limit":{"type":"integer"},
			"meta":{"type":"object","additionalProperties":{"type":"string"}}
		}}`, string(tool.InputSchema))
}

func TestNewTypedMCPToolDecodesArgumentsAndRejectsBadOnes(t *testing.T) {
	var got typedAddArgs
	tool := agent.NewTypedMCPTool("add", "", func(_ context.Context, in typedAddArgs) (agent.MCPToolResult, error) {
		got = in
		return agent.MCPToolResult{Content: []agent.MCPContent{agent.TextContent("ok")}}, nil
	})
	res, err := tool.Handler(context.Background(), json.RawMessage(`{"a":2,"b":3,"mode":"fast"}`))
	require.NoError(t, err)
	assert.Equal(t, "ok", res.Content[0].Text)
	assert.Equal(t, typedAddArgs{A: 2, B: 3, Mode: "fast"}, got)

	_, err = tool.Handler(context.Background(), json.RawMessage(`{"a":"x"}`))
	assert.ErrorContains(t, err, "invalid arguments")
}

func TestNewTypedMCPToolRefusesATypeItCannotDescribe(t *testing.T) {
	type bad struct {
		F func() `json:"f"`
	}
	assert.PanicsWithValue(t, `agent: tool "x": field F: unsupported type func()`, func() {
		agent.NewTypedMCPTool("x", "", func(context.Context, bad) (agent.MCPToolResult, error) { return agent.MCPToolResult{}, nil })
	})
	assert.Panics(t, func() {
		agent.NewTypedMCPTool("x", "", func(context.Context, int) (agent.MCPToolResult, error) { return agent.MCPToolResult{}, nil })
	})
}

func TestNewTypedMCPToolRefusesARecursiveType(t *testing.T) {
	type node struct {
		Next *node `json:"next"`
	}
	assert.Panics(t, func() {
		agent.NewTypedMCPTool("x", "", func(context.Context, node) (agent.MCPToolResult, error) { return agent.MCPToolResult{}, nil })
	})
}

func TestNewTypedMCPToolDescribesATimeAsADateTimeString(t *testing.T) {
	type args struct {
		At time.Time `json:"at"`
	}
	tool := agent.NewTypedMCPTool("x", "", func(context.Context, args) (agent.MCPToolResult, error) { return agent.MCPToolResult{}, nil })
	assert.JSONEq(t, `{"type":"object","required":["at"],"properties":{"at":{"type":"string","format":"date-time"}}}`, string(tool.InputSchema))
}
