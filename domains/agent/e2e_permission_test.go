package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

// A permission handler that could only say yes or no could not rewrite a
// tool's input, accept the CLI's "always allow" suggestion, or stop the turn.
// These drive the stub CLI over real pipes: it asks, and reports back the
// answer it read off the wire.

// reportedAnswer runs a query and returns the text the stub CLI reported: the
// control_response body it read from the SDK.
func reportedAnswer(t *testing.T, opts agent.Options) string {
	t.Helper()
	var answer string
	for _, m := range collect(t, agent.Query(context.Background(), "clean", opts)) {
		if m.Assistant != nil {
			for _, b := range m.Assistant.Content {
				if b.Text != nil {
					answer = b.Text.Text
				}
			}
		}
	}
	return answer
}

func askPermission(t *testing.T, fn agent.ToolPermissionFunc) (map[string]any, agent.ToolPermissionRequest) {
	t.Helper()
	var seen agent.ToolPermissionRequest
	opts := fakeOpts("old-cli-permission-request", nil)
	opts.ToolPermissionFunc = func(ctx context.Context, req agent.ToolPermissionRequest) agent.PermissionResult {
		seen = req
		return fn(ctx, req)
	}
	answer := reportedAnswer(t, opts)
	require.NotEmpty(t, answer, "the stub CLI reported no answer")
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(answer), &out), answer)
	return out, seen
}

func TestE2E_PermissionFuncSeesTheContextAndMaySuggestRulesBack(t *testing.T) {
	out, seen := askPermission(t, func(_ context.Context, req agent.ToolPermissionRequest) agent.PermissionResult {
		return agent.PermissionResult{Allow: true, UpdatedPermissions: req.Suggestions}
	})

	assert.Equal(t, "Bash", seen.ToolName)
	assert.Equal(t, "toolu_perm", seen.ToolUseID)
	assert.JSONEq(t, `{"command":"rm -rf build"}`, string(seen.Input))
	assert.Equal(t, "agent-7", seen.AgentID)
	assert.Equal(t, "/work/build", seen.BlockedPath)
	assert.Equal(t, "hook asked", seen.DecisionReason)
	assert.Equal(t, "Claude wants to run rm -rf build", seen.Title)
	assert.Equal(t, "Run command", seen.DisplayName)
	assert.Equal(t, "Deletes build/", seen.Description)
	require.Len(t, seen.Suggestions, 1)
	assert.Equal(t, "addRules", seen.Suggestions[0].Type)

	assert.Equal(t, "allow", out["behavior"])
	assert.Equal(t, map[string]any{"command": "rm -rf build"}, out["updatedInput"],
		"an allow with no new input must repeat the original, as upstream's SDK does")
	assert.Equal(t, []any{map[string]any{
		"type":        "addRules",
		"rules":       []any{map[string]any{"toolName": "Bash", "ruleContent": "rm -rf build:*"}},
		"behavior":    "allow",
		"destination": "session",
	}}, out["updatedPermissions"])
}

func TestE2E_PermissionFuncCanRewriteTheToolInput(t *testing.T) {
	out, _ := askPermission(t, func(context.Context, agent.ToolPermissionRequest) agent.PermissionResult {
		return agent.PermissionResult{Allow: true, UpdatedInput: json.RawMessage(`{"command":"rm -rf build --dry-run"}`)}
	})
	assert.Equal(t, "allow", out["behavior"])
	assert.Equal(t, map[string]any{"command": "rm -rf build --dry-run"}, out["updatedInput"])
	assert.NotContains(t, out, "updatedPermissions")
}

func TestE2E_PermissionFuncDenyCanInterruptTheTurn(t *testing.T) {
	out, _ := askPermission(t, func(context.Context, agent.ToolPermissionRequest) agent.PermissionResult {
		return agent.PermissionResult{Message: "not in this repo", Interrupt: true}
	})
	assert.Equal(t, map[string]any{"behavior": "deny", "message": "not in this repo", "interrupt": true}, out)
}

// ToolPermissionHandler keeps its old behaviour when the new function is unset.
func TestE2E_ToolPermissionHandlerAnswerIsUnchanged(t *testing.T) {
	opts := fakeOpts("old-cli-permission-request", nil)
	opts.ToolPermissionHandler = func(context.Context, string, json.RawMessage, string) (bool, string) {
		return false, "no"
	}
	answer := reportedAnswer(t, opts)
	assert.JSONEq(t, `{"behavior":"deny","message":"no"}`, answer)
}
