# Architecture

This document explains the key design decisions behind the Go SDK and how the components fit together.

## Overview

The SDK wraps the `claude` CLI binary as a subprocess. All communication happens over the process's stdin and stdout as newline-delimited JSON, using a bidirectional "control protocol" layered on top of the message stream.

```
User code
    │
    ├── agent.Query(ctx, prompt, opts)   OR   agent.Client
    │
    ▼
sessionManager
    │  goroutines + channels
    │
    ▼
transport.Transport (interface)
    │
    ▼
subprocess.Transport (real)  ─── stdin  ──▶  claude CLI
                             ◀── stdout ───
```

## Concurrency model

Python's async/await maps naturally onto Go goroutines and channels.

One **read-loop goroutine** runs for the lifetime of each session. It:

1. Calls `Transport.Receive()` in a loop
2. Parses each line with `protocol.ParseLine()`
3. Routes the result:
   - `control_response` → resolves a pending outbound request via `sync.Map`
   - `control_request` → dispatches to a callback in a new goroutine
   - `control_cancel_request` → closes the pending-request channel
   - `end` / `error` / EOF → closes the message channel and signals the error channel
   - All other types → converts to `messages.Message` and sends to the message channel

User-facing code reads from the message channel via the `iter.Seq2` iterator returned by `Query` or `Client.Query`.

### Request/response correlation

Outbound control requests (interrupt, set_permission_mode, etc.) need to be matched to responses. The `sessionManager` uses:

```go
pendingCtrl sync.Map   // requestID(string) → chan controlResult(capacity 1)
```

`sendControlRequest` registers a channel, sends the JSON, then blocks on the channel (with context cancellation). The read loop resolves it when a matching `control_response` arrives.

### Write serialization

The `jsonlines.Writer` holds a `sync.Mutex`. This ensures that concurrent writes from the main goroutine (user messages) and callback goroutines (permission/hook responses) never interleave on stdin.

## Layers

### `shared/jsonlines`

The lowest layer. Wraps `bufio.Scanner` (read) and `io.Writer` + mutex (write). Has no knowledge of JSON semantics — it just frames bytes with newlines.

The scanner uses `bufio.ScanLines` with a 10 MB max token buffer. This is intentionally large because assistant messages can contain entire file contents.

Python's SDK accumulated partial lines speculatively because `anyio.TextReceiveStream` could truncate long lines. Go's `bufio.Scanner` with `ScanLines` grows its internal buffer to fit a full line, so no speculative accumulation is needed.

### `domains/protocol`

Wire-level types and pure conversion functions. The types mirror the exact JSON shape emitted and consumed by the CLI:

- `types.go` — all wire structs with `json` tags
- `parser.go` — `ParseLine([]byte) (*WireMessage, error)`: two-pass decode (envelope type, then full struct)
- `builder.go` — `BuildXxx(...)  ([]byte, error)`: pure marshal functions with no I/O

Unknown message types produce an empty `*WireMessage{}` (all fields nil). This is intentional forward-compatibility: a future CLI version can emit new message types without crashing older SDK versions.

### `domains/messages`

Clean public types, decoupled from wire JSON shapes. The `FromWire(*protocol.WireMessage) (*Message, error)` function is the only bridge between the two layers.

Key conversion behaviour:
- A `content` field that arrives as a plain JSON `string` is normalised into `[]ContentBlock{{Text: &TextBlock{...}}}`. This handles older or simplified CLI output.
- Unknown content block types produce an empty `TextBlock` (forward-compat).

### `domains/transport`

A 3-method interface:

```go
type Transport interface {
    Send(ctx context.Context, data []byte) error
    Receive(ctx context.Context) ([]byte, error)
    Close() error
}
```

This is the **primary test seam**. The `subprocess.Transport` is the only production implementation; tests inject a `FakeTransport` struct. Separating `Send` and `Receive` into independent methods lets the read-loop goroutine block on `Receive` while the main goroutine calls `Send` concurrently.

### `domains/transport/subprocess`

Builds the CLI command, manages the OS process lifecycle, and exposes the `transport.Transport` interface.

**CLI invocation always uses:**
```
claude --output-format stream-json --verbose [option flags...] --input-format stream-json
```

`--input-format stream-json` is always placed last. Both flags are always present, regardless of `Options` content.

**Environment filtering:** `CLAUDECODE` is stripped from the inherited environment so SDK-spawned subprocesses do not misidentify themselves as being inside a Claude Code parent process (mirrors the Python SDK's same behaviour at #573).

**Shutdown sequence:** `Close()` uses `sync.Once` to prevent double-close:
1. Close stdin (signals EOF to the CLI)
2. Wait 5 seconds for graceful exit
3. Send SIGTERM; wait 5 more seconds
4. Send SIGKILL; wait for exit

**CLI discovery order:**
1. Explicit `Config.CLIPath`
2. `exec.LookPath("claude")`
3. Well-known paths: `~/.npm-global/bin/claude`, `/usr/local/bin/claude`, `~/.local/bin/claude`, `~/node_modules/.bin/claude`, `~/.claude/local/claude`

### `domains/agent`

The orchestration layer. Three exported types:

| Type | Use case |
|---|---|
| `agent.Query` | One-shot, one-prompt, auto-cleanup |
| `agent.Client` | Stateful, multi-turn, single subprocess |
| `agent.Options` | Configuration for either |

`sessionManager` is unexported and shared between the two. It owns the read-loop goroutine, the `sync.Map` of pending control requests, and the hook/permission callback maps.

**Default tool permission behaviour:** when no `ToolPermissionHandler` is set and the CLI sends a `can_use_tool` control request, the SDK defaults to **allow**. This matches the Python SDK's default.

**Hook key renaming:** the Python SDK uses `async_` and `continue_` as map keys to avoid Python reserved words. The Go SDK preserves this for wire compatibility: if a `HookHandler` returns a map with `async_` or `continue_` keys, they are renamed to `async` and `continue` before being sent to the CLI.

### `domains/sessions`

Session management operates directly on the JSONL session files that the Claude CLI writes under `~/.claude/projects/`. The `Store` interface decouples the operations from the filesystem implementation, enabling testing with a `FakeStore`.

The `FilesystemStore` uses a sidecar `.meta.json` file for mutable metadata (title, tags) rather than appending to the JSONL file. This avoids the need to parse and rewrite the entire session history for simple rename/tag operations.

## The control protocol

The CLI and SDK exchange two-way control messages interleaved with the regular message stream.

**SDK → CLI (outbound requests):**

```json
{"type":"control_request","request_id":"req_1_a1b2c3d4","request":{"subtype":"initialize","hooks":{},"agents":{}}}
{"type":"control_request","request_id":"req_2_e5f6a7b8","request":{"subtype":"interrupt"}}
{"type":"control_request","request_id":"req_3_c9d0e1f2","request":{"subtype":"set_permission_mode","mode":"acceptEdits"}}
```

**CLI → SDK (inbound requests, require a response):**

```json
{"type":"control_request","request_id":"req_cli_1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{...},"tool_use_id":"toolu_01"}}
{"type":"control_request","request_id":"req_cli_2","request":{"subtype":"hook_callback","callback_id":"hook_0","input":{...}}}
```

**SDK → CLI (response to inbound):**

```json
{"type":"control_response","response":{"subtype":"success","request_id":"req_cli_1","response":{"behavior":"allow"}}}
```

**CLI → SDK (response to outbound):**

```json
{"type":"control_response","response":{"subtype":"success","request_id":"req_1_a1b2c3d4","response":{}}}
```

The `control_cancel_request` message cancels an in-flight inbound control request if the CLI decides it no longer needs the answer.

### Initialize handshake

The very first message on stdin must always be an `initialize` control request. This is sent before the user prompt and carries:
- `hooks`: a map of hook event names to lists of callback registrations (matcher + callbackID + timeout)
- `agents`: agent definitions (currently always empty `{}` in this SDK)

The SDK blocks in `initialize()` until the CLI responds with a success control response (or the context times out after 60 seconds).

Only after a successful initialize does the SDK write the user prompt message.

## Why iter.Seq2

Go 1.23 introduced `iter.Seq2[K, V]` as the standard type for range-over-function iterators. Using it gives callers the familiar `for msg, err := range ...` syntax with no channels or goroutines visible at the call site. The channel-based implementation is entirely hidden behind the iterator.

A fallback to `(<-chan Message, <-chan error)` was considered but rejected: managing two channels at the call site is more error-prone and less idiomatic than a simple range loop.

## Why no DI framework

The `agent` package is a library, not a server. Dependency injection frameworks (`samber/do`, `wire`, etc.) are appropriate for application binaries where you need to assemble a large dependency graph at startup. Here, all dependencies flow through the `Options` struct, which is a plain value type. The only extension points (`Transport`, `Store`, `ToolPermissionHandler`, `HookHandler`) are injected as fields or function values — idiomatic Go without a framework.

## Testing strategy

Every package has a defined test seam:

| Package | Seam | What it replaces |
|---|---|---|
| `domains/agent` | `Options.Transport` → `FakeTransport` | subprocess, network, filesystem |
| `domains/sessions` | `Store` interface → `FakeStore` | filesystem |
| `domains/protocol` | pure functions | nothing to mock |
| `domains/messages` | pure functions | nothing to mock |
| `domains/transport/subprocess` | `RUN_INTEGRATION=1` guard | real CLI binary |

All unit tests pass with `go test ./...` on a machine without the `claude` binary installed.
