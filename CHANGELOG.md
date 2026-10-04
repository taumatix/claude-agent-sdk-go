# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/taumatix/claude-agent-sdk-go/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.7.0
[0.6.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.6.0
[0.5.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.5.0
[0.4.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.4.0
[0.3.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.3.0
[0.2.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.2.0
[0.1.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.1.0
