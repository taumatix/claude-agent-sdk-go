# Claude Agent SDK for Go

A Go SDK for [Claude Code](https://claude.ai/code) that lets you run Claude as a programmable agent in your Go applications. This is a Go port of the official [Python SDK](https://github.com/anthropics/claude-agent-sdk).

The SDK wraps the `claude` CLI binary as a subprocess and communicates over newline-delimited JSON streams, exposing a clean Go API with Go 1.23 range iterators.

> **Upstream: ported from `claude-agent-sdk-python` at [`566e41f`](https://github.com/anthropics/claude-agent-sdk-python/commit/566e41f7a59377885693082d0e8436d8964a0491) (2026-03-30), which is 476 commits behind upstream as of 2026-10-03 — a six-month gap that is still widening.**
> The feature surface is the Python SDK as it stood in March 2026 — a test cannot fail for a
> feature that was never ported. Version 0.2.0 also shipped a real bug the green suite could not
> see: every server-side tool result was discarded, because the SDK matched a wire type the CLI
> does not send. **Fixed in v0.3.0 — upgrade if you are on 0.2.0.** See [UPSTREAM.md](UPSTREAM.md)
> for what the gap means for you, and [ROADMAP.md](ROADMAP.md) for how it is being closed.

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

Every `messages.Message` has exactly one non-nil *transport* field — `User`, `Assistant`,
`System`, `Result`, `StreamEvent`, `RateLimit` or `ConvReset`:

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

### Following a subagent

Some `system` messages also arrive decoded. `task_started`, `task_progress`, `task_updated`,
`task_notification` and the three hook phases populate a typed field **in addition to** `System`,
so the switch above keeps working unchanged — put the typed cases first if you want them:

```go
for msg, err := range agent.Query(ctx, prompt, opts) {
    if err != nil { log.Fatal(err) }

    switch {
    case msg.TaskStarted != nil:
        fmt.Printf("[task %s started: %s]\n",
            msg.TaskStarted.TaskID, msg.TaskStarted.SubagentType)

    case msg.TaskProgress != nil:
        fmt.Printf("[task %s: %s, %d tokens]\n", msg.TaskProgress.TaskID,
            msg.TaskProgress.LastToolName, msg.TaskProgress.Usage.TotalTokens)

    case msg.TaskUpdated != nil && msg.TaskUpdated.Status.IsTerminal():
        fmt.Printf("[task %s finished: %s]\n",
            msg.TaskUpdated.TaskID, msg.TaskUpdated.Status)

    case msg.HookEvent != nil:
        fmt.Printf("[hook %s %s]\n",
            msg.HookEvent.HookEventName, msg.HookEvent.Phase)

    case msg.System != nil:
        // Every other subtype, with its payload in Raw.
        fmt.Printf("[system: %s]\n", msg.System.Subtype)
    }
}
```

Two things worth knowing before you track tasks:

- **A terminal state can arrive only as `task_updated`.** A task stopped by the host reports
  `TaskStatusKilled` in its patch and the matching `task_notification` is sometimes suppressed.
  Clear active-task state on `Status.IsTerminal()` from *either* message — the two spell the same
  transition differently (`killed` versus `stopped`), which is what `IsTerminal` is for.
- **`SystemMessage.Data` is deprecated and always nil.** It is bound to a `data` key that no
  `claude` release up to 2.1.267 emits; every subtype puts its fields at the top level. Read
  `SystemMessage.Raw`, which carries the whole message, for any subtype this SDK does not model.

### "Is anything still running?"

Pairing `TaskStarted` with terminal statuses works until one bookend goes missing, and then an
indicator is stuck on "running". Two level signals answer the question directly:

```go
opts := agent.DefaultOptions()
// session_state_changed is opt-in; the CLI sends none without this.
opts.Env = map[string]string{"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS": "1"}

var background []messages.BackgroundTask // reset whenever the CLI process starts
for msg, err := range agent.Query(ctx, prompt, opts) {
    if err != nil { log.Fatal(err) }

    switch {
    case msg.BackgroundTasksChanged != nil:
        // The full set, every time: replace, never merge.
        background = msg.BackgroundTasksChanged.Tasks

    case msg.SessionStateChanged != nil:
        fmt.Println("[session]", msg.SessionStateChanged.State) // idle, running, requires_action
    }
}
```

`background_tasks_changed` lists **background** tasks only: a subagent running in the foreground
is absent until it is backgrounded. `idle` is the CLI's authoritative "no further turn is owed",
and it arrives *after* the `Result`.

### When a `Query` ends

At the first `Result` only if the CLI has nothing else to say. A background task that finishes
after that result wakes the session for a follow-up turn, and its hooks and permission requests
need the CLI's stdin. So the SDK asks the CLI for session state
(`CLAUDE_CODE_SDK_READS_SESSION_STATE`, frames it keeps from you) and a `Query` runs until the CLI
reports `idle`. You may therefore see more than one `Result` in one `Query`, the follow-up turn's
included. If you opted in to session state yourself, `idle` is the `Query`'s last message.

- A CLI that reports no state (2.1.283 does not honour the SDK's request) gives the SDK only the
  task frames to go on. A `Query` stays open while a **background agent** it saw start
  (`task_type` `local_agent` or `local_workflow`) has not reported finishing, and ends at the
  `Result` that follows. Background shells and monitors are not waited for, because they can run
  for ever. With your own `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS` opt-in, 2.1.283 reports state
  anyway, and the `Query` also waits for `idle`.
- The wait after a `Result` is bounded by `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS` (default 600000,
  ten minutes), the same variable that bounds the CLI's own wait for background work. A tracked
  agent still running is not cut off by it, so bound a `Query` that may launch agents with your
  own context deadline.
- Stopping the iteration early (`break`) still ends the `Query` at once.

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
