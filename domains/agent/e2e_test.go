package agent_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	client := agent.NewClient(agent.Options{CLIPath: fakeCLIPath})
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
