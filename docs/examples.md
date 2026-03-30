# Examples

All examples assume:

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "github.com/taumatix/claude-agent-sdk-go/domains/agent"
    "github.com/taumatix/claude-agent-sdk-go/domains/messages"
)
```

---

## Basic query

```go
ctx := context.Background()

for msg, err := range agent.Query(ctx, "What is 2+2?", agent.DefaultOptions()) {
    if err != nil {
        log.Fatal(err)
    }
    if msg.Assistant != nil {
        for _, block := range msg.Assistant.Content {
            if block.Text != nil {
                fmt.Print(block.Text.Text)
            }
        }
    }
}
```

---

## Collect all text from a response

```go
func collectText(ctx context.Context, prompt string) (string, error) {
    var sb strings.Builder
    for msg, err := range agent.Query(ctx, prompt, agent.DefaultOptions()) {
        if err != nil {
            return "", err
        }
        if msg.Assistant != nil {
            for _, block := range msg.Assistant.Content {
                if block.Text != nil {
                    sb.WriteString(block.Text.Text)
                }
            }
        }
    }
    return sb.String(), nil
}
```

---

## Custom model and permission mode

```go
maxTurns := 5

opts := agent.DefaultOptions()
opts.Model = "claude-opus-4-6"
opts.MaxTurns = &maxTurns
opts.PermissionMode = agent.PermissionModeAcceptEdits
opts.WorkingDirectory = "/my/project"

for msg, err := range agent.Query(ctx, "Refactor main.go to use structured logging", opts) {
    if err != nil {
        log.Fatal(err)
    }
    if msg.Assistant != nil {
        for _, block := range msg.Assistant.Content {
            if block.Text != nil {
                fmt.Print(block.Text.Text)
            }
            if block.ToolUse != nil {
                fmt.Printf("\n[running tool: %s]\n", block.ToolUse.Name)
            }
        }
    }
    if msg.Result != nil && msg.Result.TotalCostUSD != nil {
        fmt.Printf("\nCost: $%.4f across %d turns\n", *msg.Result.TotalCostUSD, msg.Result.NumTurns)
    }
}
```

---

## Multi-turn session with Client

```go
client := agent.NewClient(agent.DefaultOptions())
if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer client.Disconnect()

questions := []string{
    "List the files in the current directory",
    "Which of those are Go source files?",
    "What does the largest one do?",
}

for _, q := range questions {
    fmt.Printf("\nQ: %s\nA: ", q)
    for msg, err := range client.Query(ctx, q) {
        if err != nil {
            log.Fatal(err)
        }
        if msg.Assistant != nil {
            for _, block := range msg.Assistant.Content {
                if block.Text != nil {
                    fmt.Print(block.Text.Text)
                }
            }
        }
    }
}
```

---

## Tool permission callback

```go
opts := agent.DefaultOptions()
opts.ToolPermissionHandler = func(
    ctx       context.Context,
    toolName  string,
    input     json.RawMessage,
    toolUseID string,
) (allow bool, reason string) {
    // Block all Bash commands containing "rm"
    if toolName == "Bash" {
        var in struct {
            Command string `json:"command"`
        }
        if err := json.Unmarshal(input, &in); err == nil {
            if strings.Contains(in.Command, "rm") {
                return false, "destructive commands are not permitted"
            }
        }
    }
    // Log all tool uses and allow everything else
    fmt.Printf("[tool: %s]\n", toolName)
    return true, ""
}

for msg, err := range agent.Query(ctx, "Clean up temp files", opts) {
    if err != nil {
        log.Fatal(err)
    }
    _ = msg
}
```

---

## Hook handlers

```go
opts := agent.DefaultOptions()
opts.HookHandlers = map[string][]agent.HookMatcher{
    // Fire before every Bash command
    "PreToolUse": {
        {
            Matcher: "Bash",
            Handler: func(ctx context.Context, id string, input json.RawMessage) (map[string]interface{}, error) {
                var in struct{ Command string `json:"command"` }
                json.Unmarshal(input, &in)
                fmt.Printf("[pre-hook] bash: %s\n", in.Command)
                return map[string]interface{}{"continue_": true}, nil
            },
        },
    },
    // Fire when the session ends
    "Stop": {
        {
            Handler: func(ctx context.Context, id string, input json.RawMessage) (map[string]interface{}, error) {
                fmt.Println("[hook] session ended")
                return nil, nil
            },
        },
    },
}

for msg, err := range agent.Query(ctx, "List Go files", opts) {
    if err != nil {
        log.Fatal(err)
    }
    _ = msg
}
```

---

## MCP server

```go
opts := agent.DefaultOptions()
opts.MCPServers = []agent.MCPServerConfig{
    {
        Name:    "my-api",
        Type:    "stdio",
        Command: "my-mcp-server",
        Args:    []string{"--verbose"},
        Env:     map[string]string{"API_KEY": os.Getenv("MY_API_KEY")},
    },
}

for msg, err := range agent.Query(ctx, "Use the my-api server to fetch the latest data", opts) {
    if err != nil {
        log.Fatal(err)
    }
    _ = msg
}
```

---

## Session continuation

```go
// First session: capture the session ID from the result
var sessionID string

opts := agent.DefaultOptions()
for msg, err := range agent.Query(ctx, "Write a haiku about Go", opts) {
    if err != nil {
        log.Fatal(err)
    }
    if msg.Result != nil {
        sessionID = msg.Result.SessionID
    }
}

// Second session: continue from where we left off
opts2 := agent.DefaultOptions()
opts2.ResumeSessionID = sessionID

for msg, err := range agent.Query(ctx, "Now write one about Rust", opts2) {
    if err != nil {
        log.Fatal(err)
    }
    if msg.Assistant != nil {
        for _, block := range msg.Assistant.Content {
            if block.Text != nil {
                fmt.Print(block.Text.Text)
            }
        }
    }
}
```

---

## Interrupt a running session

```go
client := agent.NewClient(agent.DefaultOptions())
client.Connect(ctx)
defer client.Disconnect()

// Start a long-running query in a goroutine
go func() {
    for msg, err := range client.Query(ctx, "Analyse every file in this repo") {
        if err != nil {
            return
        }
        _ = msg
    }
}()

// Interrupt after 2 seconds
time.Sleep(2 * time.Second)
if err := client.Interrupt(ctx); err != nil {
    log.Println("interrupt error:", err)
}
```

---

## Extended thinking

```go
budget := 8000

opts := agent.DefaultOptions()
opts.MaxThinkingTokens = &budget
// or: opts.Effort = "high"

for msg, err := range agent.Query(ctx, "Solve this logic puzzle: ...", opts) {
    if err != nil {
        log.Fatal(err)
    }
    if msg.Assistant != nil {
        for _, block := range msg.Assistant.Content {
            switch {
            case block.Thinking != nil:
                fmt.Println("[thinking]", block.Thinking.Thinking)
            case block.Text != nil:
                fmt.Println("[answer]", block.Text.Text)
            }
        }
    }
}
```

---

## Error handling

```go
import (
    sdkerrors "github.com/taumatix/claude-agent-sdk-go/domains/errors"
    "errors"
)

for msg, err := range agent.Query(ctx, prompt, agent.DefaultOptions()) {
    if err != nil {
        var notFound *sdkerrors.CLINotFoundError
        var procErr  *sdkerrors.ProcessError
        var initErr  *sdkerrors.InitializeError
        var jsonErr  *sdkerrors.CLIJSONDecodeError

        switch {
        case errors.As(err, &notFound):
            fmt.Println("Claude CLI not found.")
            fmt.Println("Install: npm install -g @anthropic-ai/claude-code")
        case errors.As(err, &procErr):
            fmt.Printf("Process exited %d\n", procErr.ExitCode)
            if procErr.Stderr != "" {
                fmt.Println("stderr:", procErr.Stderr)
            }
        case errors.As(err, &initErr):
            fmt.Println("Handshake failed:", initErr.Msg)
        case errors.As(err, &jsonErr):
            fmt.Println("JSON decode error:", jsonErr.Msg)
            fmt.Println("Caused by:", errors.Unwrap(err))
        default:
            fmt.Println("Unexpected error:", err)
        }
        return
    }
    _ = msg
}
```

---

## Session management

```go
import "github.com/taumatix/claude-agent-sdk-go/domains/sessions"

store, err := sessions.NewFilesystemStore("") // defaults to ~/.claude/projects
if err != nil {
    log.Fatal(err)
}

// List all sessions
list, err := sessions.ListSessions(store, "")
if err != nil {
    log.Fatal(err)
}
for _, s := range list {
    fmt.Printf("%s  %s\n", s.SessionID, s.Summary)
}

// Get a session's messages
info, msgs, err := sessions.GetSession(store, sessionID)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Session: %s (%d messages)\n", info.Summary, len(msgs))

// Rename and tag
sessions.RenameSession(store, sessionID, "Refactor session 2025-03")
sessions.TagSession(store, sessionID, "archived")

// Fork at message 10
newID, err := sessions.ForkSession(store, sessionID, 10)
fmt.Println("Forked into:", newID)

// Delete
sessions.DeleteSession(store, sessionID)
```

---

## Custom transport for testing

```go
// Implement transport.Transport to test code that drives the SDK.
type SequentialTransport struct {
    msgs [][]byte
    i    int
    mu   sync.Mutex
}

func (t *SequentialTransport) Send(_ context.Context, _ []byte) error { return nil }
func (t *SequentialTransport) Receive(_ context.Context) ([]byte, error) {
    t.mu.Lock()
    defer t.mu.Unlock()
    if t.i >= len(t.msgs) {
        time.Sleep(time.Hour) // block indefinitely
    }
    b := t.msgs[t.i]
    t.i++
    return b, nil
}
func (t *SequentialTransport) Close() error { return nil }

// Build wire messages manually for test fixtures
func wireAssistant(text string) []byte {
    b, _ := json.Marshal(map[string]interface{}{
        "type": "assistant",
        "session_id": "test",
        "message": map[string]interface{}{
            "role":    "assistant",
            "content": []map[string]interface{}{{"type": "text", "text": text}},
        },
        "model": "claude-test",
    })
    return b
}

func wireResult() []byte {
    b, _ := json.Marshal(map[string]interface{}{
        "type": "result", "subtype": "success",
        "session_id": "test", "is_error": false,
        "num_turns": 1, "duration_ms": 100,
    })
    return b
}

// Usage in a test
func TestMyCode(t *testing.T) {
    // ... handle initialize handshake, then serve messages ...
    // See domains/agent/client_test.go for the full autoInitTransport helper.
    opts := agent.DefaultOptions()
    opts.Transport = &SequentialTransport{
        msgs: [][]byte{
            wireInitResponse(initReqID),
            wireAssistant("Hello!"),
            wireResult(),
        },
    }
    // ...
}
```

See `domains/agent/client_test.go` for the complete `autoInitTransport` helper that handles the initialize handshake automatically.
