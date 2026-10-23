package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

func TestHookOutputMap(t *testing.T) {
	yes := true
	cases := []struct {
		name string
		out  agent.HookJSONOutput
		want string
	}{
		{"empty sync", agent.SyncHookJSONOutput{}, `{}`},
		{"async", agent.AsyncHookJSONOutput{AsyncTimeout: 500}, `{"async":true,"asyncTimeout":500}`},
		{"async no timeout", agent.AsyncHookJSONOutput{}, `{"async":true}`},
		{"continue true is kept", agent.SyncHookJSONOutput{Continue: &yes, Decision: "block", Reason: "r", SuppressOutput: true},
			`{"continue":true,"decision":"block","reason":"r","suppressOutput":true}`},
		{"pretooluse", agent.SyncHookJSONOutput{HookSpecificOutput: agent.PreToolUseHookSpecificOutput{
			PermissionDecision: "deny", PermissionDecisionReason: "no", UpdatedInput: map[string]interface{}{"command": "ls"}}},
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"no","updatedInput":{"command":"ls"}}}`},
		{"posttooluse output", agent.SyncHookJSONOutput{HookSpecificOutput: agent.PostToolUseHookSpecificOutput{UpdatedToolOutput: map[string]interface{}{"stdout": "x"}}},
			`{"hookSpecificOutput":{"hookEventName":"PostToolUse","updatedToolOutput":{"stdout":"x"}}}`},
		{"permission request", agent.SyncHookJSONOutput{HookSpecificOutput: agent.PermissionRequestHookSpecificOutput{Decision: map[string]interface{}{"behavior": "allow"}}},
			`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`},
		{"session start", agent.SyncHookJSONOutput{HookSpecificOutput: agent.SessionStartHookSpecificOutput{WatchPaths: []string{"/a"}, ReloadSkills: true}},
			`{"hookSpecificOutput":{"hookEventName":"SessionStart","watchPaths":["/a"],"reloadSkills":true}}`},
		{"worktree path is required", agent.SyncHookJSONOutput{HookSpecificOutput: agent.WorktreeCreateHookSpecificOutput{}},
			`{"hookSpecificOutput":{"hookEventName":"WorktreeCreate","worktreePath":""}}`},
		{"elicitation", agent.SyncHookJSONOutput{HookSpecificOutput: agent.ElicitationHookSpecificOutput{Action: "accept", Content: map[string]interface{}{"k": 1}}},
			`{"hookSpecificOutput":{"hookEventName":"Elicitation","action":"accept","content":{"k":1}}}`},
		{"message display", agent.SyncHookJSONOutput{HookSpecificOutput: agent.MessageDisplayHookSpecificOutput{DisplayContent: "d"}},
			`{"hookSpecificOutput":{"hookEventName":"MessageDisplay","displayContent":"d"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.JSONEq(t, c.want, mustJSON(t, c.out.Map()))
		})
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
