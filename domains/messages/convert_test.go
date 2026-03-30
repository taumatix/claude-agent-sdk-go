package messages_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

func wireMsg(t *testing.T, data string) *protocol.WireMessage {
	t.Helper()
	msg, err := protocol.ParseLine([]byte(data))
	require.NoError(t, err)
	return msg
}

func TestFromWire_Nil(t *testing.T) {
	// Unknown wire message (all nil) → returns nil, nil
	w := &protocol.WireMessage{}
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	assert.Nil(t, msg)
}

func TestFromWire_UserWithStringContent(t *testing.T) {
	w := wireMsg(t, `{"type":"user","session_id":"s1","message":{"role":"user","content":"hello"}}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.User)
	require.Len(t, msg.User.Content, 1)
	require.NotNil(t, msg.User.Content[0].Text)
	assert.Equal(t, "hello", msg.User.Content[0].Text.Text)
}

func TestFromWire_AssistantWithTextBlock(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[{"type":"text","text":"Hi there!"}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	assert.Equal(t, "claude-3", msg.Assistant.Model)
	require.Len(t, msg.Assistant.Content, 1)
	require.NotNil(t, msg.Assistant.Content[0].Text)
	assert.Equal(t, "Hi there!", msg.Assistant.Content[0].Text.Text)
}

func TestFromWire_AssistantWithThinkingBlock(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"I think...","signature":"sig123"}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.Len(t, msg.Assistant.Content, 1)
	require.NotNil(t, msg.Assistant.Content[0].Thinking)
	assert.Equal(t, "I think...", msg.Assistant.Content[0].Thinking.Thinking)
	assert.Equal(t, "sig123", msg.Assistant.Content[0].Thinking.Signature)
}

func TestFromWire_AssistantWithToolUse(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"ls"}}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.Len(t, msg.Assistant.Content, 1)
	require.NotNil(t, msg.Assistant.Content[0].ToolUse)
	assert.Equal(t, "tu1", msg.Assistant.Content[0].ToolUse.ID)
	assert.Equal(t, "Bash", msg.Assistant.Content[0].ToolUse.Name)
}

func TestFromWire_UserWithToolResult(t *testing.T) {
	isErr := false
	_ = isErr
	w := wireMsg(t, `{"type":"user","session_id":"s1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"result text","is_error":false}]}}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.User)
	require.Len(t, msg.User.Content, 1)
	require.NotNil(t, msg.User.Content[0].ToolResult)
	assert.Equal(t, "tu1", msg.User.Content[0].ToolResult.ToolUseID)
	assert.False(t, msg.User.Content[0].ToolResult.IsError)
}

func TestFromWire_SystemMessage(t *testing.T) {
	w := wireMsg(t, `{"type":"system","subtype":"task_started","session_id":"s1","task_id":"t1","uuid":"u1"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.System)
	assert.Equal(t, "task_started", msg.System.Subtype)
	assert.Equal(t, "t1", msg.System.TaskID)
}

func TestFromWire_ResultMessage(t *testing.T) {
	w := wireMsg(t, `{"type":"result","subtype":"success","session_id":"s1","duration_ms":1000,"duration_api_ms":700,"is_error":false,"num_turns":1}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Result)
	assert.Equal(t, "success", msg.Result.Subtype)
	assert.False(t, msg.Result.IsError)
	assert.Equal(t, int64(1000), msg.Result.DurationMS)
}

func TestFromWire_StreamEvent(t *testing.T) {
	w := wireMsg(t, `{"type":"stream_event","uuid":"u1","session_id":"s1","event":{"type":"content_block_start"}}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.StreamEvent)
	assert.Equal(t, "u1", msg.StreamEvent.UUID)
}

func TestFromWire_RateLimitEvent(t *testing.T) {
	w := wireMsg(t, `{"type":"rate_limit_event","uuid":"u1","session_id":"s1","rate_limit_info":{"status":"allowed_warning"}}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.RateLimit)
	assert.Equal(t, "allowed_warning", msg.RateLimit.RateLimitInfo.Status)
}

func TestFromWire_MultipleContentBlocks(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[{"type":"text","text":"Hello"},{"type":"tool_use","id":"tu1","name":"Read","input":{"path":"/tmp"}}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.Len(t, msg.Assistant.Content, 2)
	assert.NotNil(t, msg.Assistant.Content[0].Text)
	assert.NotNil(t, msg.Assistant.Content[1].ToolUse)
}

func TestFromWire_AssistantWithParentToolUseID(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[{"type":"text","text":"sub-agent"}]},"model":"claude-3","parent_tool_use_id":"toolu_parent"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.NotNil(t, msg.Assistant.ParentToolUseID)
	assert.Equal(t, "toolu_parent", *msg.Assistant.ParentToolUseID)
}

func TestFromWire_ResultWithCost(t *testing.T) {
	w := wireMsg(t, `{"type":"result","subtype":"success","session_id":"s1","duration_ms":500,"duration_api_ms":300,"is_error":false,"num_turns":1,"total_cost_usd":0.003}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Result)
	require.NotNil(t, msg.Result.TotalCostUSD)
	assert.InDelta(t, 0.003, *msg.Result.TotalCostUSD, 0.0001)
}

func TestFromWire_AssistantWithUsage(t *testing.T) {
	usage := json.RawMessage(`{"input_tokens":100,"output_tokens":50}`)
	_ = usage
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]},"model":"claude-3","usage":{"input_tokens":100,"output_tokens":50}}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	assert.NotNil(t, msg.Assistant.Usage)
}
