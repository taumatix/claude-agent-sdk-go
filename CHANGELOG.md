# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/taumatix/claude-agent-sdk-go/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.2.0
[0.1.0]: https://github.com/taumatix/claude-agent-sdk-go/releases/tag/v0.1.0
