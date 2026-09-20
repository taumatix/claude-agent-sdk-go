# Claude Agent SDK for Go

A Go SDK for [Claude Code](https://claude.ai/code) that lets you run Claude as a programmable agent in your Go applications. This is a Go port of the official [Python SDK](https://github.com/anthropics/claude-agent-sdk).

The SDK wraps the `claude` CLI binary as a subprocess and communicates over newline-delimited JSON streams, exposing a clean Go API with Go 1.23 range iterators.

> **Upstream: ported from `claude-agent-sdk-python` at [`566e41f`](https://github.com/anthropics/claude-agent-sdk-python/commit/566e41f7a59377885693082d0e8436d8964a0491) (2026-03-30), which is 447 commits behind upstream as of 2026-09-20.**
> The feature surface is the Python SDK as it stood in March 2026 — a test cannot fail for a
> feature that was never ported. Version 0.2.0 also shipped a real bug the green suite could not
> see: every server-side tool result was discarded, because the SDK matched a wire type the CLI
> does not send. That is fixed on `main` and unreleased. See [UPSTREAM.md](UPSTREAM.md) for what
> the gap means for you, and [ROADMAP.md](ROADMAP.md) for how it is being closed.

## Requirements

- Go 1.23+
- [Claude Code CLI](https://claude.ai/code) installed and on your `PATH`

## Installation

```bash
go get github.com/taumatix/claude-agent-sdk-go
```

## Quick start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/taumatix/claude-agent-sdk-go/domains/agent"
    "github.com/taumatix/claude-agent-sdk-go/domains/messages"
)

func main() {
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
        if msg.Result != nil {
            fmt.Printf("\nDone in %dms\n", msg.Result.DurationMS)
        }
    }
}
```

Run the bundled example:

```bash
go run ./cmd/example "Explain Go interfaces in one sentence."
```

## Usage

### One-shot queries

`agent.Query` is the simplest entry point. It spawns a `claude` subprocess, sends the prompt, streams all response messages, and cleans up automatically.

```go
opts := agent.DefaultOptions()
opts.Model = "claude-opus-4-6"
opts.MaxTurns = intPtr(3)
opts.PermissionMode = agent.PermissionModeAcceptEdits

for msg, err := range agent.Query(ctx, "Refactor this function to be more idiomatic", opts) {
    if err != nil {
        log.Fatal(err)
    }
    switch {
    case msg.Assistant != nil:
        printBlocks(msg.Assistant.Content)
    case msg.Result != nil:
        fmt.Printf("Cost: $%.6f\n", *msg.Result.TotalCostUSD)
    }
}
```

Iteration ends when either a `Result` message is yielded (clean completion) or an error occurs. Always check `err` on each iteration.

### Stateful multi-turn sessions

Use `agent.Client` when you need a persistent connection across multiple prompts:

```go
client := agent.NewClient(agent.DefaultOptions())
if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer client.Disconnect()

for _, prompt := range []string{
    "What files are in the current directory?",
    "Which of those are Go source files?",
    "Summarise what the largest one does.",
} {
    for msg, err := range client.Query(ctx, prompt) {
        if err != nil {
            log.Fatal(err)
        }
        if msg.Assistant != nil {
            printBlocks(msg.Assistant.Content)
        }
    }
}
```

### Handling message types

Every `messages.Message` has exactly one non-nil field:

```go
for msg, err := range agent.Query(ctx, prompt, opts) {
    if err != nil { log.Fatal(err) }

    switch {
    case msg.User != nil:
        fmt.Println("[user]", textContent(msg.User.Content))

    case msg.Assistant != nil:
        for _, block := range msg.Assistant.Content {
            switch {
            case block.Text != nil:
                fmt.Print(block.Text.Text)
            case block.ToolUse != nil:
                fmt.Printf("[tool: %s]\n", block.ToolUse.Name)
            case block.Thinking != nil:
                fmt.Println("[thinking]", block.Thinking.Thinking)
            }
        }

    case msg.System != nil:
        fmt.Printf("[system: %s]\n", msg.System.Subtype)

    case msg.Result != nil:
        fmt.Printf("[done] turns=%d cost=$%.4f\n",
            msg.Result.NumTurns, *msg.Result.TotalCostUSD)

    case msg.RateLimit != nil:
        fmt.Println("[rate limit]", msg.RateLimit.RateLimitInfo.Status)
    }
}
```

### Tool permission callbacks

Intercept and approve or deny tool calls at runtime:

```go
opts := agent.DefaultOptions()
opts.ToolPermissionHandler = func(
    ctx       context.Context,
    toolName  string,
    input     json.RawMessage,
    toolUseID string,
) (allow bool, reason string) {
    if toolName == "Bash" {
        var in struct{ Command string `json:"command"` }
        json.Unmarshal(input, &in)
        if strings.Contains(in.Command, "rm") {
            return false, "destructive commands are not allowed"
        }
    }
    return true, ""
}
```

### Hook handlers

Register callbacks for lifecycle events (`PreToolUse`, `PostToolUse`, `Stop`, etc.):

```go
opts := agent.DefaultOptions()
opts.HookHandlers = map[string][]agent.HookMatcher{
    "PreToolUse": {
        {
            Matcher: "Bash",   // only fire for Bash tool; "" matches all
            Handler: func(ctx context.Context, callbackID string, input json.RawMessage) (map[string]interface{}, error) {
                fmt.Println("about to run Bash tool")
                return map[string]interface{}{"continue_": true}, nil
            },
        },
    },
    "Stop": {
        {
            Handler: func(ctx context.Context, callbackID string, input json.RawMessage) (map[string]interface{}, error) {
                fmt.Println("session ended")
                return nil, nil
            },
        },
    },
}
```

The map key is any [Claude Code hook event name](https://docs.anthropic.com/en/docs/claude-code/hooks). The `continue_` key in the response map is automatically renamed to `continue` before being sent to the CLI (Go reserved-word workaround).

### MCP servers

Attach external MCP servers to a session:

```go
opts := agent.DefaultOptions()
opts.MCPServers = []agent.MCPServerConfig{
    {
        Name:    "my-tools",
        Type:    "stdio",
        Command: "my-mcp-server",
        Args:    []string{"--port", "8080"},
        Env:     map[string]string{"API_KEY": os.Getenv("API_KEY")},
    },
}
```

HTTP/SSE servers are also supported (`Type: "http"` or `Type: "sse"` with a `URL`).

### Session management

```go
import "github.com/taumatix/claude-agent-sdk-go/domains/sessions"

store, _ := sessions.NewFilesystemStore("")  // defaults to ~/.claude/projects

// List all sessions
list, _ := sessions.ListSessions(store, "")

// Read a specific session
info, msgs, _ := sessions.GetSession(store, sessionID)

// Rename / tag
sessions.RenameSession(store, sessionID, "My important session")
sessions.TagSession(store, sessionID, "archived")

// Fork at a specific message index
newID, _ := sessions.ForkSession(store, sessionID, 5)

// Delete
sessions.DeleteSession(store, sessionID)
```

### Error handling

```go
import (
    sdkerrors "github.com/taumatix/claude-agent-sdk-go/domains/errors"
    "errors"
)

for msg, err := range agent.Query(ctx, prompt, opts) {
    if err != nil {
        var notFound *sdkerrors.CLINotFoundError
        var procErr  *sdkerrors.ProcessError
        var initErr  *sdkerrors.InitializeError

        switch {
        case errors.As(err, &notFound):
            fmt.Println("claude CLI not found — install with: npm install -g @anthropic-ai/claude-code")
        case errors.As(err, &procErr):
            fmt.Printf("process exited %d: %s\n", procErr.ExitCode, procErr.Stderr)
        case errors.As(err, &initErr):
            fmt.Println("handshake failed:", initErr.Msg)
        default:
            log.Fatal(err)
        }
        return
    }
}
```

### Custom transport (testing)

Inject a fake transport to test code that drives the SDK without spawning a real process:

```go
type FakeTransport struct {
    messages [][]byte
    idx      int
}

func (f *FakeTransport) Send(_ context.Context, _ []byte) error { return nil }
func (f *FakeTransport) Receive(_ context.Context) ([]byte, error) {
    if f.idx >= len(f.messages) {
        <-make(chan struct{}) // block
    }
    b := f.messages[f.idx]; f.idx++
    return b, nil
}
func (f *FakeTransport) Close() error { return nil }

// In tests:
opts := agent.DefaultOptions()
opts.Transport = &FakeTransport{messages: [][]byte{...}}
```

See `domains/agent/client_test.go` for a complete worked example with automatic initialize-handshake handling.

### Runtime controls

```go
client := agent.NewClient(opts)
client.Connect(ctx)

// Interrupt the currently running generation
client.Interrupt(ctx)

// Change permission mode mid-session
client.SetPermissionMode(ctx, agent.PermissionModeAcceptEdits)

// Swap models mid-session
client.SetModel(ctx, "claude-haiku-4-5")
```

## Configuration reference

See [docs/api.md](docs/api.md) for the full API reference.

See [docs/architecture.md](docs/architecture.md) for architectural decisions and internals.

## Project structure

```
domains/
  agent/           # Public API — Query(), Client, Options, callbacks
  messages/        # Message and ContentBlock types
  transport/       # Transport interface
    subprocess/    # Real CLI subprocess transport
  sessions/        # Session management (list, rename, tag, fork, delete)
  errors/          # Typed error hierarchy
shared/
  jsonlines/       # Newline-delimited JSON I/O
  reqid/           # Unique request ID generation
cmd/
  example/         # Runnable example
```

## Versioning and stability

This module follows [Semantic Versioning](https://semver.org/). While the major version is `0`,
the public API is still settling, but breaking changes will not be made casually:

- Additive changes (new functions, new option fields) land in minor releases.
- Any incompatible change to an exported symbol requires a minor bump while `0.x`, and is
  called out in [CHANGELOG.md](CHANGELOG.md) with the migration path.
- Every pull request runs `apidiff` against its base and fails on incompatible public API
  changes, so a break is always a deliberate decision rather than an accident.

Pin a version in your `go.mod` and read the changelog before upgrading.

## License

MIT — see [LICENSE](LICENSE).

This project is a Go port of the official [Claude Agent SDK for Python](https://github.com/anthropics/claude-agent-sdk) by Anthropic, PBC.
