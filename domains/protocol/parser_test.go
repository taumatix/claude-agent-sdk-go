package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkerrors "github.com/taumatix/claude-agent-sdk-go/domains/errors"
	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

func TestParseLine_UserMessage(t *testing.T) {
	data := []byte(`{"type":"user","session_id":"sess_1","message":{"role":"user","content":"hello"},"uuid":"u1"}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.User)
	assert.Equal(t, protocol.TypeUser, msg.User.Type)
	assert.Equal(t, "sess_1", msg.User.SessionID)
	assert.Equal(t, "user", msg.User.Message.Role)
	require.NotNil(t, msg.User.UUID)
	assert.Equal(t, "u1", *msg.User.UUID)
}

func TestParseLine_AssistantMessage(t *testing.T) {
	data := []byte(`{"type":"assistant","session_id":"sess_1","message":{"role":"assistant","content":[{"type":"text","text":"Hello!"}]},"model":"claude-3"}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	assert.Equal(t, protocol.TypeAssistant, msg.Assistant.Type)
	assert.Equal(t, "claude-3", msg.Assistant.Model)
}

func TestParseLine_SystemMessage(t *testing.T) {
	data := []byte(`{"type":"system","subtype":"task_started","session_id":"sess_1","task_id":"task_1","uuid":"uuid_1"}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.System)
	assert.Equal(t, "task_started", msg.System.Subtype)
	assert.Equal(t, "task_1", msg.System.TaskID)
}

func TestParseLine_ResultMessage(t *testing.T) {
	costUSD := 0.005
	data := []byte(`{"type":"result","subtype":"success","session_id":"sess_1","duration_ms":1234,"duration_api_ms":800,"is_error":false,"num_turns":2,"total_cost_usd":0.005}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.Result)
	assert.Equal(t, "success", msg.Result.Subtype)
	assert.Equal(t, int64(1234), msg.Result.DurationMS)
	assert.False(t, msg.Result.IsError)
	assert.Equal(t, 2, msg.Result.NumTurns)
	require.NotNil(t, msg.Result.TotalCostUSD)
	assert.InDelta(t, costUSD, *msg.Result.TotalCostUSD, 0.0001)
}

func TestParseLine_StreamEvent(t *testing.T) {
	data := []byte(`{"type":"stream_event","uuid":"u1","session_id":"sess_1","event":{"type":"content_block_start"}}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.StreamEvent)
	assert.Equal(t, "u1", msg.StreamEvent.UUID)
}

func TestParseLine_RateLimitEvent(t *testing.T) {
	data := []byte(`{"type":"rate_limit_event","uuid":"u1","session_id":"sess_1","rate_limit_info":{"status":"allowed_warning"}}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.RateLimitEvent)
	assert.Equal(t, "allowed_warning", msg.RateLimitEvent.RateLimitInfo.Status)
}

// conversation_reset is a top-level message type, not a system subtype — see
// claude-agent-sdk-python v0.2.152, _internal/message_parser.py case
// "conversation_reset". Before this case existed the SDK fell through to the
// forward-compat default and dropped the message.
func TestParseLine_ConversationReset(t *testing.T) {
	data := []byte(`{"type":"conversation_reset","new_conversation_id":"conv_2","uuid":"u1","session_id":"sess_1"}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.ConvReset)
	assert.Equal(t, "conv_2", msg.ConvReset.NewConversationID)
	assert.Equal(t, "u1", msg.ConvReset.UUID)
	assert.Equal(t, "sess_1", msg.ConvReset.SessionID)
}

// Field names and semantics follow claude-agent-sdk-python v0.2.152
// types.py ResultMessage.
func TestParseLine_ResultMessageExtendedFields(t *testing.T) {
	data := []byte(`{"type":"result","subtype":"success","session_id":"sess_1",` +
		`"duration_ms":1234,"duration_api_ms":800,"is_error":true,"num_turns":2,` +
		`"terminal_reason":"aborted_streaming","api_error_status":429,` +
		`"structured_output":{"ok":true},"model_usage":{"claude-3":{"provider":"anthropic"}},` +
		`"permission_denials":[{"tool_name":"Bash"}],"errors":["overloaded"],` +
		`"origin":{"kind":"task-notification"}}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.Result)

	assert.Equal(t, int64(800), msg.Result.DurationAPIMS)
	require.NotNil(t, msg.Result.TerminalReason)
	assert.Equal(t, "aborted_streaming", *msg.Result.TerminalReason)
	require.NotNil(t, msg.Result.APIErrorStatus)
	assert.Equal(t, 429, *msg.Result.APIErrorStatus)
	assert.JSONEq(t, `{"ok":true}`, string(msg.Result.StructuredOutput))
	assert.JSONEq(t, `{"claude-3":{"provider":"anthropic"}}`, string(msg.Result.ModelUsage))
	assert.JSONEq(t, `[{"tool_name":"Bash"}]`, string(msg.Result.PermissionDenials))
	assert.Equal(t, []string{"overloaded"}, msg.Result.Errors)
	assert.JSONEq(t, `{"kind":"task-notification"}`, string(msg.Result.Origin))
}

// A result from an older CLI carries none of the extended fields; they must
// stay zero rather than fail the parse.
func TestParseLine_ResultMessageWithoutExtendedFields(t *testing.T) {
	data := []byte(`{"type":"result","subtype":"success","session_id":"sess_1","duration_ms":10,"duration_api_ms":5,"is_error":false,"num_turns":1}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.Result)
	assert.Nil(t, msg.Result.TerminalReason)
	assert.Nil(t, msg.Result.APIErrorStatus)
	assert.Nil(t, msg.Result.StructuredOutput)
	assert.Nil(t, msg.Result.Errors)
	assert.Nil(t, msg.Result.Origin)
}

func TestParseLine_ControlRequest(t *testing.T) {
	data := []byte(`{"type":"control_request","request_id":"req_1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{},"tool_use_id":"tu_1"}}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.ControlRequest)
	assert.Equal(t, "req_1", msg.ControlRequest.RequestID)
}

func TestParseLine_ControlResponse(t *testing.T) {
	data := []byte(`{"type":"control_response","response":{"subtype":"success","request_id":"req_1","response":{}}}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.ControlResponse)
	assert.Equal(t, "success", msg.ControlResponse.Response.Subtype)
	assert.Equal(t, "req_1", msg.ControlResponse.Response.RequestID)
}

func TestParseLine_ControlCancelRequest(t *testing.T) {
	data := []byte(`{"type":"control_cancel_request","request_id":"req_1"}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.ControlCancel)
	assert.Equal(t, "req_1", msg.ControlCancel.RequestID)
}

func TestParseLine_EndMessage(t *testing.T) {
	data := []byte(`{"type":"end"}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.End)
}

func TestParseLine_ErrorMessage(t *testing.T) {
	data := []byte(`{"type":"error","message":"something went wrong"}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg.Error)
	assert.Equal(t, "something went wrong", msg.Error.Message)
}

func TestParseLine_UnknownTypeForwardCompat(t *testing.T) {
	data := []byte(`{"type":"future_unknown_type","foo":"bar"}`)
	msg, err := protocol.ParseLine(data)
	require.NoError(t, err)
	require.NotNil(t, msg)
	// All fields nil — forward compatible
	assert.Nil(t, msg.User)
	assert.Nil(t, msg.Assistant)
	assert.Nil(t, msg.System)
	assert.Nil(t, msg.Result)
	assert.Nil(t, msg.End)
	assert.Nil(t, msg.Error)
}

func TestParseLine_InvalidJSON(t *testing.T) {
	data := []byte(`not valid json`)
	msg, err := protocol.ParseLine(data)
	assert.Nil(t, msg)
	require.Error(t, err)
	var decodeErr *sdkerrors.CLIJSONDecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, decodeErr.Error(), "failed to decode CLI JSON")
}

func TestParseControlRequestBody_CanUseTool(t *testing.T) {
	env := &protocol.ControlRequestEnvelope{
		Type:      protocol.TypeControlRequest,
		RequestID: "req_1",
		Request:   json.RawMessage(`{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls"},"tool_use_id":"tu_1"}`),
	}
	body, err := protocol.ParseControlRequestBody(env)
	require.NoError(t, err)
	req, ok := body.(*protocol.CanUseToolRequest)
	require.True(t, ok)
	assert.Equal(t, "Bash", req.ToolName)
	assert.Equal(t, "tu_1", req.ToolUseID)
}

func TestParseControlRequestBody_HookCallback(t *testing.T) {
	env := &protocol.ControlRequestEnvelope{
		Type:      protocol.TypeControlRequest,
		RequestID: "req_2",
		Request:   json.RawMessage(`{"subtype":"hook_callback","callback_id":"hook_0","input":{"hook_event_name":"PreToolUse"}}`),
	}
	body, err := protocol.ParseControlRequestBody(env)
	require.NoError(t, err)
	req, ok := body.(*protocol.HookCallbackRequest)
	require.True(t, ok)
	assert.Equal(t, "hook_0", req.CallbackID)
}

func TestParseControlRequestBody_Initialize(t *testing.T) {
	env := &protocol.ControlRequestEnvelope{
		Type:      protocol.TypeControlRequest,
		RequestID: "req_3",
		Request:   json.RawMessage(`{"subtype":"initialize","hooks":{},"agents":{}}`),
	}
	body, err := protocol.ParseControlRequestBody(env)
	require.NoError(t, err)
	_, ok := body.(*protocol.InitializeRequestBody)
	require.True(t, ok)
}

func TestParseControlRequestBody_Interrupt(t *testing.T) {
	env := &protocol.ControlRequestEnvelope{
		Request: json.RawMessage(`{"subtype":"interrupt"}`),
	}
	body, err := protocol.ParseControlRequestBody(env)
	require.NoError(t, err)
	_, ok := body.(*protocol.InterruptRequestBody)
	require.True(t, ok)
}

func TestParseControlRequestBody_SetPermMode(t *testing.T) {
	env := &protocol.ControlRequestEnvelope{
		Request: json.RawMessage(`{"subtype":"set_permission_mode","mode":"bypassPermissions"}`),
	}
	body, err := protocol.ParseControlRequestBody(env)
	require.NoError(t, err)
	req, ok := body.(*protocol.SetPermModeRequestBody)
	require.True(t, ok)
	assert.Equal(t, "bypassPermissions", req.Mode)
}

func TestParseControlRequestBody_UnknownSubtype(t *testing.T) {
	env := &protocol.ControlRequestEnvelope{
		Request: json.RawMessage(`{"subtype":"unknown_future_subtype"}`),
	}
	_, err := protocol.ParseControlRequestBody(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown control request subtype")
}
