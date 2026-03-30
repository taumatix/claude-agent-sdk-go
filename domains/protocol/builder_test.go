package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

func TestBuildUserMessage(t *testing.T) {
	data, err := protocol.BuildUserMessage("sess_1", "hello world", nil)
	require.NoError(t, err)

	var msg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &msg))

	assert.Equal(t, "user", msg["type"])
	assert.Equal(t, "sess_1", msg["session_id"])
	assert.Nil(t, msg["parent_tool_use_id"])

	msgBody, ok := msg["message"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "user", msgBody["role"])
	assert.Equal(t, "hello world", msgBody["content"])
}

func TestBuildUserMessage_WithParentToolUseID(t *testing.T) {
	parent := "toolu_01"
	data, err := protocol.BuildUserMessage("sess_1", "content", &parent)
	require.NoError(t, err)

	var msg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "toolu_01", msg["parent_tool_use_id"])
}

func TestBuildControlRequest(t *testing.T) {
	body := protocol.InitializeRequestBody{
		Subtype: protocol.SubtypeInitialize,
		Hooks:   map[string]interface{}{},
		Agents:  map[string]interface{}{},
	}
	data, err := protocol.BuildControlRequest("req_1", body)
	require.NoError(t, err)

	var env map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &env))

	assert.Equal(t, "control_request", env["type"])
	assert.Equal(t, "req_1", env["request_id"])
	assert.NotNil(t, env["request"])
}

func TestBuildControlResponse(t *testing.T) {
	body := protocol.CanUseToolResponseBody{Behavior: "allow"}
	data, err := protocol.BuildControlResponse("req_2", body)
	require.NoError(t, err)

	var env map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &env))

	assert.Equal(t, "control_response", env["type"])
	resp, ok := env["response"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "success", resp["subtype"])
	assert.Equal(t, "req_2", resp["request_id"])
}

func TestBuildControlErrorResponse(t *testing.T) {
	data, err := protocol.BuildControlErrorResponse("req_3", "something failed")
	require.NoError(t, err)

	var env map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &env))

	resp, ok := env["response"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "error", resp["subtype"])
	assert.Equal(t, "something failed", resp["error"])
}

func TestBuildInterruptRequest(t *testing.T) {
	data, err := protocol.BuildInterruptRequest("req_4")
	require.NoError(t, err)

	var env map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &env))

	assert.Equal(t, "control_request", env["type"])
	assert.Equal(t, "req_4", env["request_id"])

	reqRaw, err := json.Marshal(env["request"])
	require.NoError(t, err)
	var req map[string]interface{}
	require.NoError(t, json.Unmarshal(reqRaw, &req))
	assert.Equal(t, "interrupt", req["subtype"])
}

func TestBuildSetPermissionModeRequest(t *testing.T) {
	data, err := protocol.BuildSetPermissionModeRequest("req_5", "bypassPermissions")
	require.NoError(t, err)

	var env map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &env))

	reqRaw, err := json.Marshal(env["request"])
	require.NoError(t, err)
	var req map[string]interface{}
	require.NoError(t, json.Unmarshal(reqRaw, &req))
	assert.Equal(t, "set_permission_mode", req["subtype"])
	assert.Equal(t, "bypassPermissions", req["mode"])
}

func TestBuildSetModelRequest(t *testing.T) {
	data, err := protocol.BuildSetModelRequest("req_6", "claude-opus-4")
	require.NoError(t, err)

	var env map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &env))

	reqRaw, err := json.Marshal(env["request"])
	require.NoError(t, err)
	var req map[string]interface{}
	require.NoError(t, json.Unmarshal(reqRaw, &req))
	assert.Equal(t, "set_model", req["subtype"])
	assert.Equal(t, "claude-opus-4", req["model"])
}

func TestBuildControlRequest_RoundTrip(t *testing.T) {
	// Build a control request and then parse it back
	body := protocol.CanUseToolRequest{
		Subtype:   protocol.SubtypeCanUseTool,
		ToolName:  "Bash",
		Input:     json.RawMessage(`{"command":"ls"}`),
		ToolUseID: "tu_1",
	}
	data, err := protocol.BuildControlRequest("req_rt", body)
	require.NoError(t, err)

	// Parse the built message
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.ControlRequest)
	assert.Equal(t, "req_rt", msg.ControlRequest.RequestID)

	// Parse the inner body
	parsed, err := protocol.ParseControlRequestBody(msg.ControlRequest)
	require.NoError(t, err)
	canUse, ok := parsed.(*protocol.CanUseToolRequest)
	require.True(t, ok)
	assert.Equal(t, "Bash", canUse.ToolName)
}
