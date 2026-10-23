package agent_test

import (
	"bytes"
	"context"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

func allowAll(context.Context, agent.ToolPermissionRequest) agent.PermissionResult {
	return agent.PermissionResult{Allow: true}
}

func TestCanUseToolShadowedWarning_Text(t *testing.T) {
	deny := func(o agent.Options) string { return agent.CanUseToolShadowedWarning(o) }

	assert.Empty(t, deny(agent.Options{AllowedTools: []string{"Read"}}), "no callback, nothing is shadowed")
	assert.Empty(t, deny(agent.Options{ToolPermissionFunc: allowAll, AllowedTools: []string{"Bash(ls:*)", "", "(x)", "Edit(oops"}}),
		"a narrow or malformed entry shadows nothing")
	assert.Contains(t, deny(agent.Options{ToolPermissionFunc: allowAll, PermissionMode: agent.PermissionModeBypass}), "bypassPermissions")

	msg := deny(agent.Options{ToolPermissionFunc: allowAll, AllowedTools: []string{"Read", "Read()", "Write(*)", "Bash(ls:*)"}})
	assert.Contains(t, msg, "can_use_tool will not be invoked for: Read, Write. ", "deduplicated, in order")
	assert.NotContains(t, msg, "Bash")
}

// The session start logs the advisory once per distinct message per process,
// observed through a real subprocess session against the stub CLI.
func TestE2E_ShadowedPermissionCallbackIsLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	opts := fakeOpts("", nil)
	opts.ToolPermissionFunc = allowAll
	opts.AllowedTools = []string{"ShadowProbeTool"}
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		collect(t, agent.Query(ctx, "hi", opts))
		cancel()
	}
	assert.Equal(t, 1, bytes.Count(buf.Bytes(), []byte("can_use_tool will not be invoked for: ShadowProbeTool")), buf.String())

	buf.Reset()
	quiet := fakeOpts("", nil)
	quiet.AllowedTools = []string{"ShadowProbeTool2"} // no callback
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	collect(t, agent.Query(ctx, "hi", quiet))
	assert.NotContains(t, buf.String(), "can_use_tool")
}
