package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

func TestParseHookInput_EachEvent(t *testing.T) {
	base := `"session_id":"s","transcript_path":"/t","cwd":"/c"`
	cases := map[string]struct {
		json  string
		check func(t *testing.T, in agent.HookInput)
	}{
		"PreToolUse": {`{"hook_event_name":"PreToolUse",` + base + `,"tool_name":"Bash","tool_input":{"command":"ls"},"tool_use_id":"tu1","agent_id":"a"}`,
			func(t *testing.T, in agent.HookInput) {
				p := in.(*agent.PreToolUseHookInput)
				assert.Equal(t, "Bash", p.ToolName)
				assert.JSONEq(t, `{"command":"ls"}`, string(p.ToolInput))
				assert.Equal(t, "tu1", p.ToolUseID)
				assert.Equal(t, "a", p.AgentID)
			}},
		"PostToolUse": {`{"hook_event_name":"PostToolUse",` + base + `,"tool_name":"Bash","tool_input":{},"tool_response":"out","tool_use_id":"tu1","duration_ms":12}`,
			func(t *testing.T, in agent.HookInput) {
				p := in.(*agent.PostToolUseHookInput)
				assert.Equal(t, `"out"`, string(p.ToolResponse))
				assert.Equal(t, 12, p.DurationMS)
			}},
		"PostToolUseFailure": {`{"hook_event_name":"PostToolUseFailure",` + base + `,"tool_name":"Bash","tool_input":{},"tool_use_id":"t","error":"boom","is_interrupt":true}`,
			func(t *testing.T, in agent.HookInput) {
				p := in.(*agent.PostToolUseFailureHookInput)
				assert.Equal(t, "boom", p.Error)
				assert.True(t, p.IsInterrupt)
			}},
		"UserPromptSubmit": {`{"hook_event_name":"UserPromptSubmit",` + base + `,"prompt":"hi","prompt_id":"pid"}`,
			func(t *testing.T, in agent.HookInput) {
				p := in.(*agent.UserPromptSubmitHookInput)
				assert.Equal(t, "hi", p.Prompt)
				assert.Equal(t, "pid", p.PromptID)
			}},
		"Stop": {`{"hook_event_name":"Stop",` + base + `,"stop_hook_active":true,"last_assistant_message":"m"}`,
			func(t *testing.T, in agent.HookInput) {
				p := in.(*agent.StopHookInput)
				assert.True(t, p.StopHookActive)
				assert.Empty(t, p.BackgroundTasks)
			}},
		"PreCompact": {`{"hook_event_name":"PreCompact",` + base + `,"trigger":"auto","custom_instructions":null}`,
			func(t *testing.T, in agent.HookInput) {
				p := in.(*agent.PreCompactHookInput)
				assert.Equal(t, "auto", p.Trigger)
				assert.Nil(t, p.CustomInstructions)
			}},
		"Notification": {`{"hook_event_name":"Notification",` + base + `,"message":"m","title":"t","notification_type":"idle"}`,
			func(t *testing.T, in agent.HookInput) {
				p := in.(*agent.NotificationHookInput)
				assert.Equal(t, "idle", p.NotificationType)
				assert.Equal(t, "t", p.Title)
			}},
		"SubagentStart": {`{"hook_event_name":"SubagentStart",` + base + `,"agent_id":"a","agent_type":"x"}`,
			func(t *testing.T, in agent.HookInput) {
				assert.Equal(t, "a", in.(*agent.SubagentStartHookInput).AgentID)
			}},
		"PermissionRequest": {`{"hook_event_name":"PermissionRequest",` + base + `,"tool_name":"Bash","tool_input":{},"permission_suggestions":[{"type":"addRules"}]}`,
			func(t *testing.T, in agent.HookInput) {
				assert.JSONEq(t, `[{"type":"addRules"}]`, string(in.(*agent.PermissionRequestHookInput).Suggestions))
			}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in, err := agent.ParseHookInput(json.RawMessage(c.json))
			require.NoError(t, err)
			assert.Equal(t, name, in.EventName())
			c.check(t, in)
		})
	}
}

func TestParseHookInput_UnknownAndBad(t *testing.T) {
	in, err := agent.ParseHookInput(json.RawMessage(`{"hook_event_name":"FutureEvent","session_id":"s","x":1}`))
	require.NoError(t, err)
	u, ok := in.(*agent.UnknownHookInput)
	require.True(t, ok)
	assert.Equal(t, "FutureEvent", u.EventName())
	assert.Contains(t, string(u.Raw), `"x":1`)

	_, err = agent.ParseHookInput(json.RawMessage(`{"session_id":"s"}`))
	assert.Error(t, err, "no event name")
	_, err = agent.ParseHookInput(json.RawMessage(`not json`))
	assert.Error(t, err)
	_, err = agent.ParseHookInput(json.RawMessage(`{"hook_event_name":"Stop","stop_hook_active":"yes"}`))
	assert.Error(t, err, "wrong field type")
}
