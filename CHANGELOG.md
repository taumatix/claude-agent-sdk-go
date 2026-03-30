# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
