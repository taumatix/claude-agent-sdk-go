# API Reference

## Package `agent`

Import path: `github.com/taumatix/claude-agent-sdk-go/domains/agent`

The primary user-facing package. Contains the two public entry points (`Query` and `Client`), the `Options` configuration type, and callback types.

---

### `Query`

```go
func Query(ctx context.Context, prompt string, opts Options) iter.Seq2[messages.Message, error]
```

Executes a one-shot prompt. Spawns a `claude` subprocess (or uses `opts.Transport` if set), performs the initialization handshake, sends the prompt, and returns a Go 1.23 range iterator.

**Parameters**

| Parameter | Description |
|---|---|
| `ctx` | Controls the lifetime of the subprocess. Cancelling it tears down the session. |
| `prompt` | The user prompt to send. |
| `opts` | Session configuration. Use `DefaultOptions()` as a starting point. |

**Iterator behaviour**

Each `yield` call receives `(messages.Message, error)`. Exactly one of the two is meaningful per iteration:

- If `error` is non-nil, it is a terminal error; the iterator stops after yielding it.
- If `error` is nil, `messages.Message` contains one non-nil field.
- Iteration ends naturally when a `messages.Message` with a non-nil `Result` field is yielded.

**Example**

```go
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

### `Client`

```go
type Client struct { /* unexported */ }

func NewClient(opts Options) *Client
```

A stateful agent client. Maintains a single subprocess across multiple `Query` calls. Methods are safe for concurrent use.

#### `Connect`

```go
func (c *Client) Connect(ctx context.Context) error
```

Starts the subprocess and performs the initialization handshake. The client is not usable until `Connect` returns nil. Returns an error if already connected.

#### `Disconnect`

```go
func (c *Client) Disconnect() error
```

Shuts down the session. Safe to call when not connected (no-op).

#### `Query`

```go
func (c *Client) Query(ctx context.Context, prompt string) iter.Seq2[messages.Message, error]
```

Sends a prompt on the already-connected session. Returns an iterator with the same semantics as the package-level `Query`. Yields an immediate error if `Connect` has not been called.

#### `Interrupt`

```go
func (c *Client) Interrupt(ctx context.Context) error
```

Sends an interrupt control request to the running session.

#### `SetPermissionMode`

```go
func (c *Client) SetPermissionMode(ctx context.Context, mode PermissionMode) error
```

Changes the permission mode for the current session at runtime. Waits for acknowledgement from the CLI.

#### `SetModel`

```go
func (c *Client) SetModel(ctx context.Context, model string) error
```

Swaps the active model for the current session at runtime. Waits for acknowledgement from the CLI.

---

### `Options`

```go
type Options struct {
    // Session identity
    SessionID       string  // attach to or resume an existing session
    ContinueSession bool    // continue the last session (--continue)
    ResumeSessionID string  // resume a specific session by ID (--resume)
    ForkSession     bool    // fork the current session (--fork-session)

    // Model
    Model         string  // e.g. "claude-opus-4-6"
    FallbackModel string  // used when primary model is unavailable

    // Limits
    MaxTurns     *int     // nil = no limit
    MaxBudgetUSD *float64 // nil = no limit

    // System prompt (mutually exclusive: only one flag is emitted)
    SystemPrompt       string  // set a custom system prompt
    AppendSystemPrompt string  // append to the default system prompt

    // Tool control
    AllowedTools    []string  // whitelist of tool names
    DisallowedTools []string  // blacklist of tool names
    Tools           []string  // base tool set; nil = default

    // Permissions
    PermissionMode PermissionMode

    // Working context
    WorkingDirectory string    // working directory for the subprocess
    AddDirs          []string  // additional directories to expose

    // MCP servers
    MCPServers []MCPServerConfig

    // Streaming
    IncludePartialMessages bool  // expose stream_event messages (--include-partial-messages)

    // Settings
    SettingSources []string  // nil = default; empty = disable all (--setting-sources "")
    Settings       string    // JSON string or path to a settings file

    // Beta features
    Betas []string  // e.g. ["context-1m-2025-08-07"]

    // Thinking configuration
    MaxThinkingTokens *int   // nil = not set
    Effort            string // "low" | "medium" | "high" | "max"

    // Arbitrary extra CLI flags
    // Key is the flag name (without --). Empty string value = boolean flag.
    ExtraArgs map[string]string

    // In-process callbacks
    ToolPermissionHandler ToolPermissionHandler
    HookHandlers          map[string][]HookMatcher  // event name → matchers

    // Overrides
    Transport transport.Transport  // nil = spawn subprocess
    CLIPath   string               // "" = auto-detect on PATH
    Env       map[string]string    // extra env vars merged into subprocess environment
}
```

#### `DefaultOptions`

```go
func DefaultOptions() Options
```

Returns an `Options` with `ExtraArgs` and `Env` initialised to empty non-nil maps. All other fields are zero values.

#### `BuildCLIArgs`

```go
func BuildCLIArgs(opts Options) []string
```

Converts `Options` into a CLI argument slice. Always begins with `--output-format stream-json --verbose` and ends with `--input-format stream-json`. Exported for debugging and custom subprocess construction.

---

### `PermissionMode`

```go
type PermissionMode string

const (
    PermissionModeDefault     PermissionMode = "default"
    PermissionModeAcceptEdits PermissionMode = "acceptEdits"
    PermissionModePlan        PermissionMode = "plan"
    PermissionModeBypass      PermissionMode = "bypassPermissions"
)
```

Controls what tools the CLI can use without asking for permission.

| Value | Behaviour |
|---|---|
| `default` | CLI uses its own configured defaults |
| `acceptEdits` | File edits are automatically accepted |
| `plan` | Claude plans but does not execute |
| `bypassPermissions` | All tools allowed without confirmation |

---

### `ToolPermissionHandler`

```go
type ToolPermissionHandler func(
    ctx       context.Context,
    toolName  string,
    input     json.RawMessage,
    toolUseID string,
) (allow bool, reason string)
```

Called when the CLI requests permission to use a tool. If no handler is set, all tools are allowed by default.

| Return value | Effect |
|---|---|
| `true, ""` | Allow the tool call |
| `false, "reason"` | Deny with a message sent back to the CLI |

The `input` argument is the tool's input as raw JSON; unmarshal it to inspect specific fields.

---

### `HookMatcher`

```go
type HookMatcher struct {
    Matcher string        // tool name pattern (e.g. "Bash", "Write|Edit"). "" = match all.
    Handler HookHandler
    Timeout time.Duration // 0 = use CLI default
}
```

Associates a pattern with a hook handler for a named event. Registered via `Options.HookHandlers`:

```go
opts.HookHandlers = map[string][]agent.HookMatcher{
    "PreToolUse":  { {Matcher: "Bash", Handler: myHandler} },
    "PostToolUse": { {Handler: logHandler} },
}
```

Supported event names: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `UserPromptSubmit`, `Stop`, `SubagentStop`, `PreCompact`, `Notification`, `SubagentStart`, `PermissionRequest`.

---

### `HookHandler`

```go
type HookHandler func(
    ctx        context.Context,
    callbackID string,
    input      json.RawMessage,
) (map[string]interface{}, error)
```

Called when the CLI fires a registered hook event. The returned map is sent back to the CLI as the hook response. Keys `async_` and `continue_` are automatically renamed to `async` and `continue` for wire compatibility.

---

### `MCPServerConfig`

```go
type MCPServerConfig struct {
    Name    string
    Type    string            // "stdio" | "http" | "sse"
    Command string            // for stdio: executable path
    Args    []string          // for stdio: arguments
    Env     map[string]string // for stdio: environment variables
    URL     string            // for http/sse: server URL
}
```

---

## Package `messages`

Import path: `github.com/taumatix/claude-agent-sdk-go/domains/messages`

### `Message`

```go
type Message struct {
    User        *UserMessage
    Assistant   *AssistantMessage
    System      *SystemMessage
    Result      *ResultMessage
    StreamEvent *StreamEventMessage
    RateLimit   *RateLimitMessage
    ConvReset   *ConversationResetMessage
}
```

Exactly one field is non-nil. Type-switch using the field:

```go
switch {
case msg.User != nil:      // ...
case msg.Assistant != nil: // ...
case msg.System != nil:    // ...
case msg.Result != nil:    // terminal message
case msg.StreamEvent != nil: // partial stream (only with IncludePartialMessages)
case msg.RateLimit != nil: // rate limit notification
case msg.ConvReset != nil: // conversation replaced mid-session
}
```

---

### `UserMessage`

```go
type UserMessage struct {
    SessionID       string
    UUID            *string
    Content         []ContentBlock
    ParentToolUseID *string
    ToolUseResult   json.RawMessage // structured tool result; nil on ordinary turns
    Origin          json.RawMessage // provenance; nil when the CLI did not attribute it
}
```

`Origin` tells an injected turn (task notification, channel or peer message) from a human one.
Its `kind` field is the discriminator, and the CLI adds new kinds over time — treat anything
unrecognised as "not human".

---

### `ConversationResetMessage`

```go
type ConversationResetMessage struct {
    NewConversationID string
    UUID              string
    SessionID         string // the outgoing session that was reset
}
```

Emitted when the conversation is replaced without ending the connection — after `/clear`, for
instance. The reset also zeroes the running totals on subsequent `ResultMessage`s, so code
accumulating `TotalCostUSD` across a long-lived session must snapshot its tally here.

`NewConversationID` is not the `SessionID` of subsequent messages; read that from the next
message.

---

### `AssistantMessage`

```go
type AssistantMessage struct {
    SessionID       string
    UUID            *string
    Model           string
    Content         []ContentBlock
    Usage           json.RawMessage  // raw Anthropic API usage object
    StopReason      *string
    MessageID       *string
    Error           *string          // e.g. "rate_limit", "billing_error"
    ParentToolUseID *string
}
```

---

### `SystemMessage`

```go
type SystemMessage struct {
    SessionID string
    Subtype   string          // e.g. "task_started", "task_progress", "task_notification"
    Data      json.RawMessage // subtype-specific payload
    TaskID    string
    UUID      string
}
```

---

### `ResultMessage`

The terminal message of every session. Always the last message yielded by the iterator.

```go
type ResultMessage struct {
    SessionID    string
    UUID         *string
    Subtype      string    // "success" or "error"
    IsError      bool
    NumTurns     int
    DurationMS   int64
    Result       string    // text summary, if any
    TotalCostUSD *float64  // nil if cost data not available
    StopReason   *string
    Usage        json.RawMessage

    DurationAPIMS     int64           // time in API calls, a subset of DurationMS
    TerminalReason    *string         // why the loop ended; nil on older CLIs
    APIErrorStatus    *int            // HTTP status of the failing call, when IsError
    StructuredOutput  json.RawMessage
    ModelUsage        json.RawMessage // per-model usage, keyed by model name
    PermissionDenials json.RawMessage // tool calls denied during the turn
    Errors            []string
    Origin            json.RawMessage // provenance of the triggering user message
}
```

`TerminalReason` is the only way to tell an interrupted turn from a completed one:
`"aborted_streaming"` and `"aborted_tools"` mean the turn was cancelled (via `Client.Interrupt`
or an `interrupt` control request); `"completed"` and `"max_turns"` mean it ran to an end. It is
nil on CLI versions predating the field and on results that bypass the query loop, such as a
local slash command.

`APIErrorStatus` carries no message content and is safe to log.

---

### `StreamEventMessage`

Only present when `Options.IncludePartialMessages` is true.

```go
type StreamEventMessage struct {
    UUID            string
    SessionID       string
    Event           json.RawMessage  // raw Anthropic API streaming event
    ParentToolUseID *string
}
```

---

### `RateLimitMessage`

```go
type RateLimitMessage struct {
    UUID          string
    SessionID     string
    RateLimitInfo protocol.RateLimitInfo
}
```

---

### `ContentBlock`

```go
type ContentBlock struct {
    Text             *TextBlock
    Thinking         *ThinkingBlock
    ToolUse          *ToolUseBlock
    ToolResult       *ToolResultBlock
    ServerToolUse    *ServerToolUseBlock
    ServerToolResult *ServerToolResultBlock
    Unknown          *UnknownBlock
}
```

Exactly one field is non-nil.

### `ServerToolUseBlock` / `ServerToolResultBlock`

Tools the API runs server-side on the model's behalf (`web_search`, `web_fetch`, ...). They
appear alongside `ToolUse` blocks in the content stream, but you never return a result for them —
the result arrives as a `ServerToolResult` block. Branch on `Name` to know which tool ran.

```go
type ServerToolUseBlock struct {
    ID    string
    Name  string          // "web_search", "web_fetch", ...
    Input json.RawMessage
}

type ServerToolResultBlock struct {
    ToolUseID string
    Content   json.RawMessage // raw API payload; inspect its "type" to decode
    IsError   bool
    Type      protocol.ContentBlockType // which server tool produced it
}
```

There is no single `server_tool_result` type on the wire: the API names each result after the
tool that ran, so `Type` is what tells you how to decode `Content`. The names are
`web_search_tool_result`, `web_fetch_tool_result`, `advisor_tool_result`,
`code_execution_tool_result`, `bash_code_execution_tool_result`,
`text_editor_code_execution_tool_result` and `tool_search_tool_result`, each available as a
`protocol.ContentType*` constant, with `protocol.IsServerToolResult` as the predicate.

A server tool that failed generally does **not** set `IsError`. It reports the failure inside
`Content`, as a type ending in `_tool_result_error` carrying an `error_code` — so do not read
`IsError == false` as "the tool succeeded".

### `UnknownBlock`

A content block this SDK version does not model: a type the CLI added after this release, or one
no SDK decodes (`mcp_tool_use`, `mcp_tool_result`, `container_upload`, `redacted_thinking`,
`compaction`). `Raw` is the block exactly as it arrived, so you can decode it yourself without
waiting for an SDK release.

```go
type UnknownBlock struct {
    Type protocol.ContentBlockType // the block's wire "type"; empty if it carried none
    Raw  json.RawMessage           // the complete block as received
}
```

### `TextBlock`

```go
type TextBlock struct {
    Text string
}
```

### `ThinkingBlock`

Extended thinking content (requires `MaxThinkingTokens` or `Effort` to be set).

```go
type ThinkingBlock struct {
    Thinking  string
    Signature string
}
```

### `ToolUseBlock`

A tool invocation within an assistant message.

```go
type ToolUseBlock struct {
    ID    string
    Name  string
    Input json.RawMessage  // tool-specific input; unmarshal as needed
}
```

### `ToolResultBlock`

The result of a tool invocation. Appears in user messages that follow an assistant tool use.

```go
type ToolResultBlock struct {
    ToolUseID  string
    Content    []ContentBlock   // parsed inner blocks
    RawContent json.RawMessage  // original raw content field
    IsError    bool
}
```

---

## Package `errors`

Import path: `github.com/taumatix/claude-agent-sdk-go/domains/errors`

All error types implement `error`. Use `errors.As` to handle them specifically.

### `CLINotFoundError`

```go
type CLINotFoundError struct {
    Msg     string
    CLIPath string  // non-empty when an explicit path was given but not found
}
```

The `claude` binary could not be located. Install Claude Code:

```bash
npm install -g @anthropic-ai/claude-code
```

Or set `Options.CLIPath` to the explicit path.

### `CLIConnectionError`

```go
type CLIConnectionError struct {
    Msg string
}
```

Subprocess pipes could not be created or the process failed to start (e.g. bad working directory).

### `ProcessError`

```go
type ProcessError struct {
    Msg      string
    ExitCode int
    Stderr   string  // non-empty when the process wrote to stderr
}
```

The CLI subprocess exited with a non-zero code or encountered a fatal runtime error.

### `CLIJSONDecodeError`

```go
type CLIJSONDecodeError struct {
    Msg           string
    Line          []byte  // the raw line that failed
    OriginalError error
}

func (e *CLIJSONDecodeError) Unwrap() error
```

A line from the CLI's stdout could not be decoded as JSON. Supports `errors.Unwrap`.

### `MessageParseError`

```go
type MessageParseError struct {
    Msg  string
    Data map[string]any
}
```

A valid JSON line could not be converted to a typed SDK message.

### `InitializeError`

```go
type InitializeError struct {
    Msg string
}
```

The SDK–CLI initialization handshake failed (timeout, cancellation, or error response from CLI).

---

## Package `transport`

Import path: `github.com/taumatix/claude-agent-sdk-go/domains/transport`

### `Transport` interface

```go
type Transport interface {
    Send(ctx context.Context, data []byte) error
    Receive(ctx context.Context) ([]byte, error)
    Close() error
}
```

The extension point for custom transports. Inject via `Options.Transport`.

| Method | Contract |
|---|---|
| `Send` | Write one JSON line (without newline; the SDK appends `\n`). Must be safe for concurrent use. Returns an error if the transport is closed or the context is cancelled. |
| `Receive` | Return the next JSON line (without newline). Blocks until a line is available. Returns `io.EOF` when the stream ends normally. Returns any other error on failure. |
| `Close` | Shut down the transport. Safe to call multiple times. |

---

## Package `subprocess`

Import path: `github.com/taumatix/claude-agent-sdk-go/domains/transport/subprocess`

### `Config`

```go
type Config struct {
    CLIPath          string            // "" = auto-detect
    Args             []string          // CLI flag arguments (NOT including the binary path)
    Env              map[string]string // merged over inherited environment
    WorkingDirectory string
}
```

### `New`

```go
func New(ctx context.Context, cfg Config) (*Transport, error)
```

Locates the CLI binary, optionally checks its version, and starts the process. Returns `*CLINotFoundError` if the binary cannot be found, or `*CLIConnectionError` if the process cannot be started.

Version checking can be skipped by setting `CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK` in the environment.

### `Transport`

```go
type Transport struct { /* unexported */ }

func (t *Transport) Send(ctx context.Context, data []byte) error
func (t *Transport) Receive(ctx context.Context) ([]byte, error)
func (t *Transport) Close() error
```

Implements `transport.Transport`. `Close` uses `sync.Once` so it is safe to call multiple times. Shutdown sequence: close stdin → wait 5 s → SIGTERM → wait 5 s → SIGKILL.

### Constant

```go
const MinimumCLIVersion = "2.0.0"
```

Minimum supported CLI version. A warning is logged if the detected version is older; this is non-fatal.

---

## Package `sessions`

Import path: `github.com/taumatix/claude-agent-sdk-go/domains/sessions`

### `Store` interface

```go
type Store interface {
    ListSessions(projectPath string) ([]SessionInfo, error)
    ReadSession(sessionID string) ([]SessionMessage, error)
    WriteSessionMeta(sessionID string, info SessionInfo) error
    DeleteSession(sessionID string) error
    ForkSession(sessionID string, upToMessage int) (string, error)
}
```

### Operations

All operations take a `Store` as the first argument:

```go
func ListSessions(store Store, projectPath string) ([]SessionInfo, error)
func GetSession(store Store, sessionID string) (*SessionInfo, []SessionMessage, error)
func RenameSession(store Store, sessionID, newTitle string) error
func TagSession(store Store, sessionID, tag string) error
func DeleteSession(store Store, sessionID string) error
func ForkSession(store Store, sessionID string, upToMessage int) (string, error)
```

`GetSession` returns both the session metadata and its messages in one call.

`ForkSession` creates a new session containing the first `upToMessage` messages of the source session and returns the new session ID. If `upToMessage` exceeds the actual message count, it is clamped.

### `FilesystemStore`

```go
type FilesystemStore struct { /* unexported */ }

func NewFilesystemStore(baseDir string) (*FilesystemStore, error)
```

Implements `Store` using JSONL files under `~/.claude/projects/`. Pass an empty string for `baseDir` to use the default location.

### `SessionInfo`

```go
type SessionInfo struct {
    SessionID    string `json:"session_id"`
    Summary      string `json:"summary"`
    LastModified int64  `json:"last_modified"`    // milliseconds since epoch
    FileSize     int64  `json:"file_size,omitempty"`
    CustomTitle  string `json:"custom_title,omitempty"`
    FirstPrompt  string `json:"first_prompt,omitempty"`
    GitBranch    string `json:"git_branch,omitempty"`
    Cwd          string `json:"cwd,omitempty"`
    Tag          string `json:"tag,omitempty"`
    CreatedAt    int64  `json:"created_at,omitempty"` // milliseconds since epoch
}
```

### `SessionMessage`

```go
type SessionMessage struct {
    Type            string      `json:"type"`                        // "user" or "assistant"
    UUID            string      `json:"uuid"`
    SessionID       string      `json:"session_id"`
    Message         interface{} `json:"message"`
    ParentToolUseID interface{} `json:"parent_tool_use_id,omitempty"`
}
```

---

## Environment variables

| Variable | Effect |
|---|---|
| `CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK` | Skip CLI version check on subprocess start |
| `CLAUDECODE` | Stripped from subprocess environment automatically |

The following variables are always injected into the subprocess environment:

| Variable | Value |
|---|---|
| `CLAUDE_CODE_ENTRYPOINT` | `sdk-go` |
| `CLAUDE_AGENT_SDK_VERSION` | `0.3.0` |
