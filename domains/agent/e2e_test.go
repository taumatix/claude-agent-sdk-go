package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

// fakeCLIPath is the compiled testdata/fakecli, built once for the whole test
// binary by TestMain. The end-to-end tests spawn it through the ordinary
// subprocess transport, so the SDK runs its real path: a real process, real
// pipes, real newline-delimited JSON. Only the counterparty is substituted — a
// live `claude` cannot be made to emit a server-side tool result on demand, and
// needs credentials CI does not have.
var fakeCLIPath string

func TestMain(m *testing.M) {
	// No defer anywhere in here: every exit from TestMain goes through os.Exit,
	// which does not run deferred calls.
	dir, err := os.MkdirTemp("", "claude-agent-sdk-go-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: temp dir:", err)
		os.Exit(1)
	}

	fakeCLIPath = filepath.Join(dir, "fakecli")
	if runtime.GOOS == "windows" {
		fakeCLIPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", fakeCLIPath, "./testdata/fakecli/main.go")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build stub CLI: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// queryFakeCLI runs one full connect-query-disconnect against the stub CLI and
// returns every message the caller saw.
func queryFakeCLI(t *testing.T, prompt string) []messages.Message {
	t.Helper()
	return queryFakeCLIWith(t, agent.Options{CLIPath: fakeCLIPath}, prompt)
}

func queryFakeCLIWith(t *testing.T, opts agent.Options, prompt string) []messages.Message {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	client := agent.NewClient(opts)
	require.NoError(t, client.Connect(ctx))
	t.Cleanup(func() { _ = client.Disconnect() })

	var got []messages.Message
	for msg, err := range client.Query(ctx, prompt) {
		require.NoError(t, err)
		got = append(got, msg)
	}
	return got
}

// TestE2E_ServerToolResultReachesTheCaller drives the whole stack over a real
// subprocess. Before 2026-09-20 the web_search_tool_result block arrived as an
// empty TextBlock — the SDK matched an invented "server_tool_result" type — so
// a caller doing a server-side search got a turn that looked like the model had
// answered without searching.
func TestE2E_ServerToolResultReachesTheCaller(t *testing.T) {
	got := queryFakeCLI(t, "when did Go 1.26 ship?")

	var assistant *messages.AssistantMessage
	for i := range got {
		if got[i].Assistant != nil {
			assistant = got[i].Assistant
		}
	}
	require.NotNil(t, assistant, "no assistant message arrived")
	require.Len(t, assistant.Content, 4)

	require.NotNil(t, assistant.Content[0].ServerToolUse)
	assert.Equal(t, "web_search", assistant.Content[0].ServerToolUse.Name)

	result := assistant.Content[1].ServerToolResult
	require.NotNil(t, result, "the server tool result was dropped")
	assert.Equal(t, protocol.ContentTypeWebSearchToolResult, result.Type)
	assert.Equal(t, "srvtoolu_01", result.ToolUseID)
	assert.JSONEq(t,
		`[{"type":"web_search_result","title":"Go 1.26","url":"https://go.dev/doc/devel/release"}]`,
		string(result.Content))

	unknown := assistant.Content[2].Unknown
	require.NotNil(t, unknown, "the unmodelled block was dropped")
	assert.Equal(t, protocol.ContentBlockType("mcp_tool_use"), unknown.Type)
	assert.JSONEq(t,
		`{"type":"mcp_tool_use","id":"mcptoolu_01","name":"lookup","server_name":"docs","input":{"q":"iter.Seq2"}}`,
		string(unknown.Raw))

	require.NotNil(t, assistant.Content[3].Text)
	assert.Equal(t, "Go 1.26 is out.", assistant.Content[3].Text.Text)
}

// The turn carries exactly one text block. Every other block used to add an
// empty one, so a caller concatenating text saw the right string and had no way
// to notice three blocks had been destroyed to produce it.
func TestE2E_NoPhantomTextBlocks(t *testing.T) {
	got := queryFakeCLI(t, "when did Go 1.26 ship?")

	texts := 0
	for _, msg := range got {
		if msg.Assistant == nil {
			continue
		}
		for _, block := range msg.Assistant.Content {
			if block.Text != nil {
				texts++
			}
		}
	}
	assert.Equal(t, 1, texts)
}

// Disconnect used to block until the caller's context expired — forever on a
// context.Background() — because it waited for the read loop before closing the
// transport, and the CLI keeps stdout open while its stdin is open. The whole
// session is given 10s here, so a return to that ordering fails rather than
// slowing the suite down by however long the timeout happens to be.
func TestE2E_DisconnectReturnsPromptly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := agent.NewClient(agent.Options{CLIPath: fakeCLIPath})
	require.NoError(t, client.Connect(ctx))

	for _, err := range client.Query(ctx, "when did Go 1.26 ship?") {
		require.NoError(t, err)
	}

	start := time.Now()
	require.NoError(t, client.Disconnect())
	assert.Less(t, time.Since(start), 5*time.Second,
		"Disconnect blocked; it is waiting on a read the CLI will not end")
}

func TestE2E_QueryTerminatesOnResult(t *testing.T) {
	got := queryFakeCLI(t, "when did Go 1.26 ship?")

	require.NotEmpty(t, got)
	last := got[len(got)-1]
	require.NotNil(t, last.Result, "the iterator must stop on the result message")
	assert.False(t, last.Result.IsError)
	assert.Equal(t, "Go 1.26 is out.", last.Result.Result)
}

// The new flags cross the real process boundary: the stub CLI records the argv it was started
// with. Only the spelling is proven here; that the real CLI accepts each flag is from
// `claude --help` (2.1.283), not from a live session.
func TestE2E_SessionAndPluginFlagsReachTheCLI(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "argv")
	queryFakeCLIWith(t, agent.Options{
		CLIPath:              fakeCLIPath,
		Env:                  map[string]string{"FAKECLI_ARGS_FILE": argsFile},
		JSONSchema:           `{"type":"object","properties":{"name":{"type":"string"}}}`,
		NoSessionPersistence: true,
		StrictMCPConfig:      true,
		PluginDirs:           []string{"/p/a", "/p/b.zip"},
		IncludeHookEvents:    true,
	}, "hi")

	raw, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	argv := strings.Split(string(raw), "\n")
	assert.Contains(t, argv, "--no-session-persistence")
	assert.Contains(t, argv, "--strict-mcp-config")
	assert.Contains(t, argv, "--include-hook-events")
	assert.Contains(t, argv, `{"type":"object","properties":{"name":{"type":"string"}}}`)
	assert.Equal(t, 2, strings.Count(string(raw), "--plugin-dir"))
}

func TestE2E_AgentsAndPluginURLsReachTheCLI(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "argv")
	queryFakeCLIWith(t, agent.Options{
		CLIPath:              fakeCLIPath,
		Env:                  map[string]string{"FAKECLI_ARGS_FILE": argsFile},
		Agents:               map[string]agent.AgentDefinition{"reviewer": {Description: "Reviews code", Prompt: "You are a code reviewer"}},
		DisableSlashCommands: true,
		PluginURLs:           []string{"https://example.com/a.zip", "https://example.com/b.zip"},
	}, "hi")

	raw, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	argv := strings.Split(string(raw), "\n")
	assert.Contains(t, argv, `{"reviewer":{"description":"Reviews code","prompt":"You are a code reviewer"}}`)
	assert.Contains(t, argv, "--disable-slash-commands")
	assert.Equal(t, 2, strings.Count(string(raw), "--plugin-url"))
}

func TestE2E_WorktreePRAndPermissionPromptFlagsReachTheCLI(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "argv")
	queryFakeCLIWith(t, agent.Options{
		CLIPath:              fakeCLIPath,
		Env:                  map[string]string{"FAKECLI_ARGS_FILE": argsFile},
		Worktree:             true,
		WorktreeName:         "fix-a",
		FromPR:               "123",
		PermissionPromptTool: "mcp__auth__approve",
		PermissionPrompts:    "host",
		Brief:                true,
	}, "hi")

	raw, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	argv := strings.Split(string(raw), "\n")
	assert.Contains(t, argv, "--brief")
	for flag, want := range map[string]string{
		"--worktree": "fix-a", "--from-pr": "123",
		"--permission-prompt-tool": "mcp__auth__approve", "--permission-prompts": "host",
	} {
		i := -1
		for j, a := range argv {
			if a == flag {
				i = j
			}
		}
		require.GreaterOrEqual(t, i, 0, flag)
		assert.Equal(t, want, argv[i+1], flag)
	}
}

// A PreToolUse hook that answers "defer" ends the run with the tool call on the result message. Before
// DeferredToolUse the field was dropped, so a deferred turn looked like one that ended with nothing pending.
// The frame is the stub's, in upstream's shape; a live CLI has not been made to defer.
func TestE2E_DeferredToolUseReachesTheCaller(t *testing.T) {
	t.Setenv("FAKECLI_SCENARIO", "deferred")
	got := queryFakeCLI(t, "run ls")
	var res *messages.ResultMessage
	for _, m := range got {
		if m.Result != nil {
			res = m.Result
		}
	}
	require.NotNil(t, res)
	require.NotNil(t, res.DeferredToolUse)
	assert.Equal(t, "toolu_01", res.DeferredToolUse.ID)
	assert.Equal(t, "Bash", res.DeferredToolUse.Name)
	assert.JSONEq(t, `{"command":"ls -la"}`, string(res.DeferredToolUse.Input))
}

func TestE2E_ATurnWithNothingDeferredHasNoDeferredToolUse(t *testing.T) {
	for _, m := range queryFakeCLI(t, "hi") {
		if m.Result != nil {
			assert.Nil(t, m.Result.DeferredToolUse)
		}
	}
}

// The CLI sends the per-model usage as "modelUsage"; the SDK read "model_usage", so ModelUsage was always empty.
// The frame is the shape a live 2.1.283 result carries.
func TestE2E_ModelUsageReachesTheCaller(t *testing.T) {
	t.Setenv("FAKECLI_SCENARIO", "modelusage")
	var res *messages.ResultMessage
	for _, m := range queryFakeCLI(t, "hi") {
		if m.Result != nil {
			res = m.Result
		}
	}
	require.NotNil(t, res)
	var usage map[string]struct {
		InputTokens int     `json:"inputTokens"`
		CostUSD     float64 `json:"costUSD"`
	}
	require.NoError(t, json.Unmarshal(res.ModelUsage, &usage))
	assert.Equal(t, 2, usage["claude-opus-5-5"].InputTokens)
	assert.InDelta(t, 0.19, usage["claude-opus-5-5"].CostUSD, 1e-9)
}

func TestE2E_ThinkingAndSystemPromptFileReachTheCLI(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "argv")
	queryFakeCLIWith(t, agent.Options{
		CLIPath:          fakeCLIPath,
		Env:              map[string]string{"FAKECLI_ARGS_FILE": argsFile},
		Thinking:         "adaptive",
		ThinkingDisplay:  "summarized",
		SystemPromptFile: "/tmp/prompt.md",
	}, "hi")

	raw, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	argv := strings.Split(string(raw), "\n")
	for flag, want := range map[string]string{
		"--thinking": "adaptive", "--thinking-display": "summarized", "--system-prompt-file": "/tmp/prompt.md",
	} {
		i := slices.Index(argv, flag)
		require.GreaterOrEqual(t, i, 0, flag)
		assert.Equal(t, want, argv[i+1], flag)
	}
}
