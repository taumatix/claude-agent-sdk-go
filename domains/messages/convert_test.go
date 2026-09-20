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

func TestFromWire_ConversationReset(t *testing.T) {
	w := wireMsg(t, `{"type":"conversation_reset","new_conversation_id":"conv_2","uuid":"u1","session_id":"s1"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.ConvReset)
	assert.Equal(t, "conv_2", msg.ConvReset.NewConversationID)
	assert.Equal(t, "u1", msg.ConvReset.UUID)
	assert.Equal(t, "s1", msg.ConvReset.SessionID)
}

func TestFromWire_ResultExtendedFields(t *testing.T) {
	w := wireMsg(t, `{"type":"result","subtype":"success","session_id":"s1","duration_ms":1000,`+
		`"duration_api_ms":700,"is_error":true,"num_turns":1,"terminal_reason":"max_turns",`+
		`"api_error_status":529,"errors":["overloaded"],"model_usage":{"claude-3":{"provider":"anthropic"}}}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Result)
	assert.Equal(t, int64(700), msg.Result.DurationAPIMS)
	require.NotNil(t, msg.Result.TerminalReason)
	assert.Equal(t, "max_turns", *msg.Result.TerminalReason)
	require.NotNil(t, msg.Result.APIErrorStatus)
	assert.Equal(t, 529, *msg.Result.APIErrorStatus)
	assert.Equal(t, []string{"overloaded"}, msg.Result.Errors)
	assert.JSONEq(t, `{"claude-3":{"provider":"anthropic"}}`, string(msg.Result.ModelUsage))
}

// server_tool_use blocks previously fell through to the unknown-block branch
// and surfaced as an empty TextBlock, silently losing the call.
func TestFromWire_AssistantWithServerToolUse(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[`+
		`{"type":"server_tool_use","id":"stu1","name":"web_search","input":{"query":"go generics"}}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.Len(t, msg.Assistant.Content, 1)
	block := msg.Assistant.Content[0]
	assert.Nil(t, block.Text)
	require.NotNil(t, block.ServerToolUse)
	assert.Equal(t, "stu1", block.ServerToolUse.ID)
	assert.Equal(t, "web_search", block.ServerToolUse.Name)
	assert.JSONEq(t, `{"query":"go generics"}`, string(block.ServerToolUse.Input))
}

// testServerToolResultBlock drives one server-side tool result block through
// the parser. The block type names come from the content-block switch in the
// `claude` binary (2.1.220) — `web_search_tool_result`, `advisor_tool_result`
// and the rest. There is no `server_tool_result` on the wire: this SDK matched
// that invented name until 2026-09-20, so every one of these blocks fell to the
// unknown branch and surfaced as an empty TextBlock.
func testServerToolResultBlock(t *testing.T, blockType, content string) {
	t.Helper()
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[`+
		`{"type":"`+blockType+`","tool_use_id":"stu1","content":`+content+`}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.Len(t, msg.Assistant.Content, 1)
	block := msg.Assistant.Content[0]
	assert.Nil(t, block.Text, "a server tool result must not surface as text")
	assert.Nil(t, block.Unknown, "a known server tool result must not surface as unknown")
	require.NotNil(t, block.ServerToolResult)
	assert.Equal(t, "stu1", block.ServerToolResult.ToolUseID)
	assert.Equal(t, protocol.ContentBlockType(blockType), block.ServerToolResult.Type)
	assert.JSONEq(t, content, string(block.ServerToolResult.Content))
}

func TestFromWire_AssistantWithServerToolResult(t *testing.T) {
	testServerToolResultBlock(t, "web_search_tool_result", `[{"type":"web_search_result","title":"Go"}]`)
	testServerToolResultBlock(t, "web_fetch_tool_result", `{"type":"web_fetch_result","url":"https://go.dev"}`)
	testServerToolResultBlock(t, "advisor_tool_result", `{"type":"advisor_result","advice":"ship it"}`)
	testServerToolResultBlock(t, "code_execution_tool_result", `{"type":"code_execution_result","stdout":"1"}`)
	testServerToolResultBlock(t, "bash_code_execution_tool_result", `{"type":"bash_code_execution_result"}`)
	testServerToolResultBlock(t, "text_editor_code_execution_tool_result", `{"type":"text_editor_code_execution_result"}`)
	testServerToolResultBlock(t, "tool_search_tool_result", `{"type":"tool_search_result","tools":[]}`)
}

// A server tool that failed says so inside its content, not via is_error.
func TestFromWire_ServerToolResultErrorLivesInContent(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[`+
		`{"type":"advisor_tool_result","tool_use_id":"stu1",`+
		`"content":{"type":"advisor_tool_result_error","error_code":"unavailable"}}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.Len(t, msg.Assistant.Content, 1)
	block := msg.Assistant.Content[0]
	require.NotNil(t, block.ServerToolResult)
	assert.False(t, block.ServerToolResult.IsError, "the wire block carries no is_error field")
	assert.JSONEq(t, `{"type":"advisor_tool_result_error","error_code":"unavailable"}`,
		string(block.ServerToolResult.Content))
}

// The name this SDK used to expect still decodes, so a caller that built a
// fixture on it keeps working.
func TestFromWire_LegacyServerToolResultNameStillDecodes(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[`+
		`{"type":"server_tool_result","tool_use_id":"stu1","content":{"type":"web_search_result"}}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.Len(t, msg.Assistant.Content, 1)
	require.NotNil(t, msg.Assistant.Content[0].ServerToolResult)
	assert.Equal(t, "stu1", msg.Assistant.Content[0].ServerToolResult.ToolUseID)
}

// Blocks the CLI emits that no SDK models must arrive whole, not as empty text.
func testUnknownContentBlock(t *testing.T, block string) {
	t.Helper()
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[`+
		block+`]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.Assistant)
	require.Len(t, msg.Assistant.Content, 1)
	got := msg.Assistant.Content[0]
	assert.Nil(t, got.Text, "an unmodelled block must not masquerade as text")
	require.NotNil(t, got.Unknown)
	assert.JSONEq(t, block, string(got.Unknown.Raw))
}

func TestFromWire_UnknownContentBlockKeepsItsPayload(t *testing.T) {
	testUnknownContentBlock(t, `{"type":"mcp_tool_use","id":"m1","name":"search","server_name":"docs"}`)
	testUnknownContentBlock(t, `{"type":"container_upload","file_id":"f1"}`)
	testUnknownContentBlock(t, `{"type":"redacted_thinking","data":"encrypted"}`)
	testUnknownContentBlock(t, `{"type":"compaction"}`)
	testUnknownContentBlock(t, `{"type":"a_block_type_invented_after_this_release","payload":{"n":1}}`)
}

func TestFromWire_UnknownContentBlockCarriesItsType(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[`+
		`{"type":"mcp_tool_use","id":"m1"}]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.Len(t, msg.Assistant.Content, 1)
	require.NotNil(t, msg.Assistant.Content[0].Unknown)
	assert.Equal(t, protocol.ContentBlockType("mcp_tool_use"), msg.Assistant.Content[0].Unknown.Type)
}

// An assistant turn whose only block is a server tool result used to arrive
// looking like the model had said nothing.
func TestFromWire_AssistantWithOnlyServerToolResultIsNotEmpty(t *testing.T) {
	w := wireMsg(t, `{"type":"assistant","session_id":"s1","message":{"role":"assistant","content":[`+
		`{"type":"server_tool_use","id":"stu1","name":"web_search","input":{"query":"go"}},`+
		`{"type":"web_search_tool_result","tool_use_id":"stu1","content":[{"type":"web_search_result","title":"Go"}]}`+
		`]},"model":"claude-3"}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.Len(t, msg.Assistant.Content, 2)

	var text string
	for _, b := range msg.Assistant.Content {
		if b.Text != nil {
			text += b.Text.Text
		}
	}
	assert.Empty(t, text, "no text block should have been fabricated")
	require.NotNil(t, msg.Assistant.Content[0].ServerToolUse)
	require.NotNil(t, msg.Assistant.Content[1].ServerToolResult)
	assert.Equal(t, msg.Assistant.Content[0].ServerToolUse.ID,
		msg.Assistant.Content[1].ServerToolResult.ToolUseID)
}

func TestFromWire_UserWithToolUseResultAndOrigin(t *testing.T) {
	w := wireMsg(t, `{"type":"user","session_id":"s1","message":{"role":"user","content":"hi"},`+
		`"tool_use_result":{"stdout":"ok"},"origin":{"kind":"task-notification","subkind":"scheduled-trigger"}}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.User)
	assert.JSONEq(t, `{"stdout":"ok"}`, string(msg.User.ToolUseResult))
	assert.JSONEq(t, `{"kind":"task-notification","subkind":"scheduled-trigger"}`, string(msg.User.Origin))
}

// An ordinary user turn carries neither field.
func TestFromWire_UserWithoutToolUseResultOrOrigin(t *testing.T) {
	w := wireMsg(t, `{"type":"user","session_id":"s1","message":{"role":"user","content":"hi"}}`)
	msg, err := messages.FromWire(w)
	require.NoError(t, err)
	require.NotNil(t, msg.User)
	assert.Nil(t, msg.User.ToolUseResult)
	assert.Nil(t, msg.User.Origin)
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
