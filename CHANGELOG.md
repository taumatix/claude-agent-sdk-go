# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Session summaries for `SessionStore` adapters** (`domains/sessions`), ported from upstream's `fold_session_summary`:
  `FoldSessionSummary` keeps a per-session `SessionSummaryEntry` current inside `Append` (first prompt, custom and AI
  titles, last prompt, git branch, cwd, tag, creation time), the optional `SessionSummaryLister` interface returns them
  in one call, and `SummaryToSessionInfo` turns one into a `SessionInfo`. `MemorySessionStore` implements it, and
  `sessionstoretest` has the 14th contract (summaries round-trip, share `ListSessions`' clock, ignore sub-transcripts,
  vanish on delete). Purely additive; a store that does not implement it is unchanged. Timestamps are parsed as
  RFC 3339, which is stricter than Python's `fromisoformat`.
- **`sessions.ListSessionsFromStore(ctx, store, projectKey, projectPath, limit, offset)`**: lists a project's sessions
  from any `SessionStore`, newest first. With `SessionSummaryLister` it is one summary call plus one `ListSessions`, and
  `Load` runs only for sessions whose summary is missing or older than the listed mtime, and only inside the requested
  page. Without summaries it loads each session (16 at a time). Sidechain and empty sessions are dropped before
  pagination; a failed `Load` gives an empty-summary row. Purely additive.
- **`Options.Thinking`** (`--thinking adaptive|disabled`), **`Options.ThinkingDisplay`** (`--thinking-display`) and
  **`Options.SystemPromptFile`** (`--system-prompt-file`), the three flags upstream's `_build_command` sends that this
  SDK did not. `Thinking` takes the place of `MaxThinkingTokens` when set; `SystemPromptFile` is sent instead of
  `SystemPrompt`. Zero-valued, nothing changes. Each was accepted by `claude` 2.1.283 in a live turn; what
  `ThinkingDisplay` values exist is not checked.
- **`ResultMessage.DeferredToolUse`** (`ID`, `Name`, `Input`): the tool call a PreToolUse hook deferred by answering
  `"defer"`, on which the run stopped. It was dropped before, so a deferred turn looked like one with nothing
  pending. Shape from upstream's `DeferredToolUse`; tested against the stub CLI, not a live one.
- **`Options.Brief`** (`--brief`), which enables the CLI's SendUserMessage tool. Zero-valued off. Spelling from
  `claude --help` on 2.1.283; not exercised against a live session.
- **`sessions.SessionStore`**, a transcript store interface tracking upstream's (`SessionKey`, `Append`,
  `Load`; optional `SessionLister`, `SessionDeleter`, `SubkeyLister`), a `MemorySessionStore`, and the
  **`sessions/sessionstoretest`** conformance suite for adapter authors (`sessionstoretest.Run`), a port of
  contracts 1 to 13 of upstream's `session_store_conformance`. Nothing yet feeds a store from a running
  session; that is the next roadmap slice.
- **`Options.Worktree`/`WorktreeName`** (`--worktree [name]`), **`FromPR`** (`--from-pr`),
  **`PermissionPromptTool`** (`--permission-prompt-tool`) and **`PermissionPrompts`** (`--permission-prompts`).
  Zero-valued off. `FromPR` empty is not sent, since the bare flag opens an interactive picker. Spellings
  from `claude --help` on 2.1.283; not exercised against a live session.
- **`Options.Agents`** (`--agents`, a map of name to `AgentDefinition{Description, Prompt}`), **`DisableSlashCommands`**
  and **`PluginURLs`** (`--plugin-url`, repeated). `AgentDefinition` carries only the two fields the CLI's
  help documents. Spellings are from `claude --help` on 2.1.283; not exercised against a live session.
- **Five more CLI flags as typed `Options`**: `JSONSchema` (`--json-schema`), `NoSessionPersistence`,
  `StrictMCPConfig`, `PluginDirs` (`--plugin-dir`, repeated, which the `ExtraArgs` map cannot express) and
  `IncludeHookEvents`. All are zero-valued off, so nothing that runs today changes. The spellings are
  from `claude --help` on 2.1.283; none was exercised against a live session.
- **`Options.RequireMinimumCLIVersion`** (and `subprocess.Config.EnforceMinimumVersion`) turns a CLI
  older than 2.0.0 into a `*errors.CLIVersionError` (`Found`, `Required`, `CLIPath`) from `Query` and
  `Client.Connect`, instead of a log line followed by a protocol error that points at this library.
  It is off by default, so nothing that runs today stops; the default will flip in a later minor,
  announced here first. A CLI whose `-v` output cannot be read is let through either way, and
  `CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK` still disables the check.

### Fixed

- **`ResultMessage.ModelUsage` was always empty.** The CLI sends the per-model usage as `modelUsage` (captured
  from 2.1.283: `inputTokens`, `outputTokens`, `costUSD`, `contextWindow`, ...); the SDK read `model_usage`, a key
  no CLI emits, so the field never filled. It now reads either spelling, `modelUsage` first. The field stays raw
  JSON, and `protocol.ResultMessage` gains `ModelUsageCamel` (additive).
- **A cancelled `Send` no longer poisons the session when none of the line was written.** `Send`
  closed stdin on every cancellation, so a `Query` whose context timed out on a full pipe made
  every later call fail and forced a reconnect. Now the line is abandoned and the transport keeps
  working when the cancel came before the write began (another `Send` still held the pipe) or the
  write had landed no byte; a write cut off part-way still closes stdin, since the CLI has seen half
  a JSON line. An abandoned line is never delivered later. On a platform whose pipes have no write
  deadline the old behaviour (close stdin) remains.
- **`Transport.Send` now honours its context.** The subprocess transport ignored `ctx` and blocked in a
  pipe write, so a CLI that stopped reading stdin with its pipe full hung the caller, and a stuck write
  also stalled `Close`. `Send` now returns `ctx.Err()` as soon as the context is done and `Close`
  releases a blocked `Send`. A write abandoned part-way leaves the stream unusable, so stdin is closed
  and later `Send` calls fail. The contract is on the `Transport` interface.
- **`Transport.Receive` now honours its context.** The subprocess transport ignored `ctx` and blocked
  in a pipe read, so cancelling it did nothing until the CLI wrote a line or exited. It now returns
  `ctx.Err()` as soon as the context is done, without losing a line (the next `Receive` returns it),
  keeps returning `io.EOF` once the stream has ended, and `Close` unblocks a waiting `Receive`. The
  `transport.Transport` doc comment states this contract for custom implementations.

## [0.11.0] - 2026-10-07

### Added

- **`Message.CompactBoundary`: where the conversation was compacted.** When `/compact` runs, or
  the CLI compacts on its own, it sends a `compact_boundary` system message that arrived as an
  untyped `System`. `CompactBoundaryMessage` carries the trigger, the context size before and
  after (`PreTokens`, `PostTokens`) and how long it took (`Duration`), so a caller can tell why
  its context shrank. Checked live against claude 2.1.283 (the installed CLI) and a 2.1.288 frame.
  The CLI's other compaction fields are internal and stay in `System.Raw`.

## [0.10.0] - 2026-10-05

### Added

- **`Message.ThinkingTokens`: progress while the model thinks.** During a thinking phase the API
  streams nothing a caller can show, and the CLI reports a running estimate as `thinking_tokens`
  system messages. These arrived as an untyped `System`. `ThinkingTokensMessage` carries the
  running total for the current thinking block, this message's increment, and the user message
  it answers when that carried a uuid. It is an estimate for progress display, not the billed
  count. Checked live against claude 2.1.283 and 2.1.288.

### Not added, on purpose

- `task_summary` and `post_turn_summary` stay untyped. The CLI's schema marks both internal, so
  their fields carry no promise. They still arrive as `System` with `Raw`.

## [0.9.0] - 2026-10-05

### Added

- **`Message.APIRetry`: the CLI is retrying a failed API request.** A run stalled on an
  overloaded, rate-limited or unreachable API was invisible until it finished late or failed. The
  CLI reports each retry as an `api_retry` system message, which arrived only as an untyped
  `System`. `APIRetryMessage` carries the attempt and its cap, the delay before it, the HTTP
  status (0 when there was none), the kind of failure, and, for a first-byte timeout, how long
  each attempt waits. Checked live against claude 2.1.283 and 2.1.288 by pointing them at a local
  server that answers 529, which costs nothing.

## [0.8.0] - 2026-10-05

### Added

- **`Message.PermissionDenied`: a tool call the CLI refused without asking.** A deny rule,
  `dontAsk` mode, the auto-mode classifier, or an "ask" with no `CanUseTool` callback to answer it
  each refuse a call silently from the caller's side. The CLI reports each as a `permission_denied`
  system message, which arrived only as an untyped `System`. `PermissionDeniedMessage` carries the
  tool, its `ToolUseID`, which component decided (`ReasonType`), an actionable `ReasonCode` when
  there is one, and the message the model was told. `System` is still set. Checked live against
  claude 2.1.283 and 2.1.288 with a deny rule.

## [0.7.2] - 2026-10-04

### Fixed

- **A background agent whose `task_notification` was lost no longer holds a `Query`, or every
  later `Query` on a `Client`.** The SDK paired `task_started` with its end frame, so one missed
  frame kept the run open until your context ended. It now also reads `background_tasks_changed`:
  the CLI's list of every live background task, which the CLI's schema says to treat as a
  replacement set "so a missed bookend cannot wedge a stale running indicator". An agent that
  list stops naming counts as finished. Both claude 2.1.283 and 2.1.288 list running agents in it
  (checked live).

## [0.7.1] - 2026-10-04

No code change.

### Documentation

- **`Query`'s usage example returned at the first `Result`.** Since 0.6.0 a run can carry more
  than one result: a background agent that finishes afterwards wakes the session for a follow-up
  turn. Breaking out at the first abandons that turn and leaves its hooks unanswered. The example
  no longer does it.
- `Query` and `Client.Query` now say, where a caller reads it, that a background agent is never
  cut off and the context is the only bound. They also say that a `Client` keeps waiting for a
  lost agent in later Queries until it reconnects. Stub-CLI tests pin both, plus a CLI that
  reports `idle` at every turn's end.

## [0.7.0] - 2026-10-04

### Fixed

- **On a CLI that reports no session state, a `Query` still ended under a running background
  agent.** 0.6.0 made a `Query` wait for the CLI's `idle`, but `claude` 2.1.283 sends no state
  unless the caller opts in. There, a `Query` still ended at the first result while an agent
  launched in the background ran on. The agent's hook was never answered, and its follow-up turn
  went to nobody. The SDK now keeps a ledger of background agents from the task frames
  (`task_started` of type `local_agent` or `local_workflow`, until a terminal `task_notification`
  or `task_updated`). A result with one in flight does not end the `Query`, and the ceiling does
  not cut one off. This ports upstream's `DEFERRING_TASK_TYPES`. Shells and monitors are not
  tracked, because they can run for ever.

  Verified live against 2.1.283: an agent launched in the background is now seen to finish
  before the `Query` ends (two results instead of one). With the ledger emptied, the same live
  test fails with the agent still running.

### Changed

- A `Query` that launches a background agent on such a CLI now lasts until the agent finishes,
  however long that is. Give it a context deadline if that matters to you.

## [0.6.0] - 2026-10-03

### Fixed

- **A one-shot `Query` closed the CLI while it still owed a turn.** `Query` returned at the first
  `ResultMessage` and then closed the CLI's stdin. A background task that finishes after that
  result wakes the session for a follow-up turn, so a hook the CLI asked to run for it was never
  answered and an SDK MCP call failed. This is the bug upstream fixed as #1088 and #1190. On a
  `Client` the follow-up turn was not lost, but it was handed to the *next* `Query`, attributed to
  the wrong prompt. Both are reproduced by the new stub-CLI end-to-end tests.

### Changed

- **A `Query` now runs until the CLI reports `idle`**, not to the first result, when the CLI
  reports session state. The SDK asks for that state with `CLAUDE_CODE_SDK_READS_SESSION_STATE`,
  unless you set the variable yourself. Frames sent only for that request (`sdk_host_only`) are
  not delivered to you. One `Query` can therefore yield more than one `ResultMessage`. A CLI that
  reports no state ends at the result, as before. Verified live against `claude` 2.1.283, which
  ignores the request, and 2.1.288, which honours it.
- The wait after a result is bounded by `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS` (default ten
  minutes), read from `Options.Env` or the environment, as upstream does.
- With your own `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS` opt-in, `idle` now reaches you as the
  `Query`'s last message. Under 0.5.0 it arrived after the `Query` had returned.

### Known limitation

A CLI that reports `idle` at every turn's end, or reports no state at all, gives no warning of a
background agent that will wake the session later. Upstream also tracks in-flight agent tasks from
the `task_*` frames for that case. That is ROADMAP entry 0b.

## [0.5.0] - 2026-09-27

### Added

- `messages.Message.BackgroundTasksChanged`, decoded from `background_tasks_changed`: the full
  set of live background tasks after every change. It is a level signal — replace your set with
  each payload — so "is background work running" no longer depends on pairing `task_started` with
  a terminal status and never missing one. The CLI sends it with no opt-in. It lists background
  tasks only; a foreground subagent is absent until it is backgrounded.
- `messages.Message.SessionStateChanged`, decoded from `session_state_changed`, with
  `messages.SessionState` (`idle`, `running`, `requires_action`). **Opt-in:** the CLI sends it
  only when `Options.Env` sets `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1`.
- Both also still arrive as `System`, as the lifecycle messages do. A frame with no `tasks` key,
  or no `state`, degrades to `System` alone rather than being typed as empty — an empty set would
  tell a caller that all background work had stopped.

Shapes come from the zod schemas in the `claude` 2.1.283 bundle and a live run on 2026-09-27; the
live end-to-end suite passes against 2.1.283. Upstream's Python SDK does not type either subtype.

### Known limitation

`Query` and `Client.Query` return at the first `Result`. On 2.1.283 a finished background task
wakes a follow-up turn after that result, and `idle` arrives after it, so a single `Query` may
not show you `idle`. On a `Client`, those later frames stay queued and the next `Client.Query`
yields them first — read from the code, not yet observed in a test.
This predates 0.5.0 and is the top of [ROADMAP.md](ROADMAP.md).

## [0.4.0] - 2026-09-25

### Fixed

- **`messages.SystemMessage.Data` has been empty since the port was written.** It is bound to a
  `data` key that no `claude` release up to 2.1.267 emits — every `system` subtype puts its fields
  at the top level. Upstream's Python `SystemMessage.data` is the *whole message dict*; the Go port
  bound that name to a nested object that has never existed, so the payload of every system message
  was unreachable. `Data` is now **deprecated** and keeps its binding (nothing silently changes
  meaning); the new `Raw` field carries the complete message.

### Added

- `messages.SystemMessage.Raw`, the complete message as it arrived. This is how to read a subtype
  or a field this SDK does not model without waiting for a release.
- Typed task lifecycle messages: `messages.Message.TaskStarted`, `.TaskProgress`, `.TaskUpdated`
  and `.TaskNotification`, decoded from the matching `system` subtype. A caller tracking a subagent
  no longer hand-decodes anything.
- `messages.Message.HookEvent` for the `hook_started`, `hook_progress` and `hook_response`
  subtypes, with `Phase` telling them apart. **`hook_progress` is a third phase upstream's Python
  SDK does not model at all** — its parser routes only the other two, while the CLI emits progress
  from an interval timer for the duration of a hook.
- `messages.TaskStatus` with `IsTerminal()`, which spans both lifecycle vocabularies: a
  `task_updated` patch reports the raw `killed` where a `task_notification` reports the mapped
  `stopped`. A task's terminal state can arrive *only* as a `task_updated`, so a caller clearing
  active-task state must accept it from either message.
- `messages.HookPhase`, `messages.HookOutcome`, `messages.TaskPatch`, `messages.TaskUsage`, and the
  `protocol.SystemSubtype` constants and payload structs behind them.

Shapes were read from the zod schemas the `claude` 2.1.267 bundle carries per subtype, then
confirmed against a live run on 2026-09-25 that spawned a subagent. The CLI models five fields on
`task_started` that upstream Python does not (`subagent_type`, `is_backgrounded`, `spawn_depth`,
`workflow_name`, `prompt`); all are included.

**Additive, not a break.** A `system` subtype this SDK does not model still arrives as `System`
alone, and a subtype it does model populates `System` *as well as* the typed field — so existing
code that switches on `msg.System` is unaffected. A malformed lifecycle payload degrades to
`System` rather than failing the stream; upstream raises `MessageParseError` there, which lets a
progress notification kill a run.

## [0.3.0] - 2026-09-21

### Fixed

- **Server-side tool results were being discarded.** The SDK matched
  `server_tool_result` as the wire type. Nothing emits that name — the API names each result after
  the tool that ran, and the content-block switch in the `claude` binary (2.1.220) lists
  `web_search_tool_result`, `web_fetch_tool_result`, `advisor_tool_result`,
  `code_execution_tool_result`, `bash_code_execution_tool_result`,
  `text_editor_code_execution_tool_result` and `tool_search_tool_result`. The 0.2.0 entry below
  claims these blocks were fixed; they were not. Every one of them hit the unknown-block branch,
  which returned an **empty `TextBlock`** — so the payload was lost *and* the caller was handed
  text the model never wrote. An assistant turn whose only content was a web search result arrived
  looking like the model had answered with nothing.
- **`Client.Disconnect()` blocked until the caller's context expired** — forever on a
  `context.Background()`. It waited for the read loop before closing the transport, but the read is
  a blocking pipe read no context can interrupt, and the CLI holds stdout open while its stdin is
  open. Measured at 119.9s against a 120s context; now returns in milliseconds.

### Added

- `messages.ContentBlock.Unknown` (`messages.UnknownBlock`), carrying `Type` and the raw JSON of
  any block this SDK does not model — including one the CLI adds after this release. Previously
  such blocks became empty `TextBlock`s.
- `messages.ServerToolResultBlock.Type`, saying which server tool produced the block. `Content`'s
  shape differs per tool, so without it the block could not be decoded.
- `protocol.IsServerToolResult` and one `protocol.ContentType*` constant per server tool result
  type. `protocol.ContentTypeServerToolResult` is **deprecated** — it never matched a real message
  — but still decodes, so code built on it keeps working.

**Behaviour change, not an API break.** The public API is additive and `apidiff` reports no
incompatible change. But a caller that today receives an empty `TextBlock` for an unmodelled block
will receive `Unknown` instead. Nothing could usefully depend on the old value: it was
indistinguishable from real empty text.

## [0.2.0] - 2026-09-17

### Added

Parsing for CLI output the SDK received but discarded. Checked against
[claude-agent-sdk-python v0.2.152](https://github.com/anthropics/claude-agent-sdk-python/releases/tag/v0.2.152)
(bundled CLI 2.1.259). All additions are new fields and new types; nothing existing changed.

- `messages.ConversationResetMessage`, reachable as `Message.ConvReset`. `conversation_reset` is a
  top-level wire type, so it previously hit the parser's forward-compatibility default and was
  dropped. It also zeroes the running totals on later results — code accumulating
  `ResultMessage.TotalCostUSD` over a long-lived session must snapshot when it arrives.
- `messages.ServerToolUseBlock` and `messages.ServerToolResultBlock` for the `server_tool_use` /
  `server_tool_result` content blocks the API emits for server-executed tools (`web_search`,
  `web_fetch`, ...). These previously fell through to the unknown-block branch and surfaced as an
  empty `TextBlock`, silently losing the call.
- `ResultMessage`: `DurationAPIMS`, `TerminalReason`, `APIErrorStatus`, `StructuredOutput`,
  `ModelUsage`, `PermissionDenials`, `Errors`, `Origin`. `TerminalReason` is the only way to tell an
  interrupted turn (`aborted_streaming`, `aborted_tools`) from a completed one.
- `UserMessage`: `ToolUseResult` and `Origin`. `Origin` distinguishes an injected turn (task
  notification, channel or peer message) from a human one.

`Origin`, `ModelUsage`, `PermissionDenials` and `StructuredOutput` are `json.RawMessage`: their
shapes grow with the CLI, and a raw field stays forward-compatible where a struct would not.

## [0.1.0] - 2026-09-06

### Fixed

- `BuildCLIArgs`: omit `--setting-sources` flag when `SettingSources` is nil or empty (backport of `fix/empty-setting-sources-cli-flag` from the Python SDK).

### Added

- Initial release of the Claude Agent SDK for Go.
- `agent.Query` for one-shot prompt execution with Go 1.23 range iterators.
- `agent.Client` for stateful multi-turn sessions.
- Tool permission callbacks via `ToolPermissionHandler`.
- Lifecycle hook handlers (`PreToolUse`, `PostToolUse`, `Stop`, etc.).
- MCP server attachment (`MCPServerConfig`).
- Session management (`domains/sessions`): list, get, rename, tag, fork, delete.
- Typed error hierarchy (`domains/errors`): `CLINotFoundError`, `ProcessError`, `InitializeError`.
- Pluggable `Transport` interface for testing without a real subprocess.
- Subprocess transport wrapping the `claude` CLI binary.

### Changed

- CI now gates every pull request on build, `go test -race` (Go 1.23 and stable), `gofmt`, `go vet`, `staticcheck`, and an `apidiff` check that fails on incompatible public API changes.

[Unreleased]: https://github.com/taumatix/claude-agent-sdk-go/compare/v0.11.0...HEAD
[0.11.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.11.0
[0.10.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.10.0
[0.9.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.9.0
[0.8.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.8.0
[0.7.2]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.7.2
[0.7.1]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.7.1
[0.7.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.7.0
[0.6.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.6.0
[0.5.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.5.0
[0.4.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.4.0
[0.3.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.3.0
[0.2.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.2.0
[0.1.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.1.0
