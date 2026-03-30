---
name: python-backport
description: "Backport changes from the Python Claude Agent SDK to this Go SDK"
---

# /python-backport - Python SDK Backport

Detects changes in the upstream Python Claude Agent SDK since the last tracked SHA
and backports them to this Go SDK.

## Usage
```
/python-backport
```

## Context

- The file `.claude-agent-sdk-python-sha` at the repo root stores the last Python SDK
  commit SHA that was backported.
- The upstream Python SDK lives at https://github.com/anthropics/claude-agent-sdk-python.git
- This Go SDK mirrors the Python SDK's architecture and protocol semantics.

## Behavioral Flow

1. **Read current SHA**: Read `.claude-agent-sdk-python-sha` to get the last synced commit.

2. **Fetch upstream changes**: Clone or fetch the Python SDK into a temp directory
   (`/tmp/claude-agent-sdk-python`), then run:
   ```
   git log <SHA>..HEAD --oneline
   git diff <SHA>..HEAD
   ```
   to get the list of commits and the full diff since the last sync.

3. **Analyse changes**: For each changed Python file/symbol, reason about what the
   equivalent Go code is and whether it needs updating. Focus on:
   - Protocol types (`domains/protocol/types.go`) — new message types, fields, constants
   - Options / CLI args (`domains/agent/options.go`, `domains/agent/query.go`) — new options mapped to CLI flags
   - Client behaviour (`domains/agent/client.go`, `domains/agent/session_manager.go`) — new methods or changed lifecycle
   - Message types (`domains/messages/types.go`, `domains/messages/convert.go`)
   - Session / transport layers

4. **Backport**: Apply the relevant changes to the Go codebase, following:
   - Project conventions in `CLAUDE.md` and `.claude/rules/`
   - Go idioms (use iterators, not callbacks, where the Python uses generators)
   - Keep parity with the Python SDK's public API surface

5. **Run Go tests**: After applying any changes (or even if no changes were needed),
   run `go test ./...` from the repo root. If tests fail, investigate and fix before
   proceeding. Only proceed to the next step once all tests pass.

6. **Update SHA**: Write the new HEAD SHA of the Python SDK to
   `.claude-agent-sdk-python-sha`. This must happen regardless of whether any
   backport changes were made — if the SDKs are already in sync, update the SHA to
   confirm the sync point was verified.

7. **Report**: Summarise what was changed, what was skipped (Python-only concerns
   such as type stubs, packaging, docs), and any manual follow-up required.

## Key Patterns

- **Protocol parity**: Every new JSON field/type in the Python SDK should appear in
  `domains/protocol/types.go`.
- **Options parity**: Every new `--flag` surfaced through `BuildCLIArgs` in Python
  should be added to `Options` and `BuildCLIArgs` in Go.
- **No Python-isms**: Skip `.pyi` stubs, `setup.py`, `pyproject.toml`, `__init__.py`
  re-exports — these have no Go equivalent.
- **Tests**: For each backported behaviour, update or add tests following the patterns
  in `.claude/rules/go_test_rule.md`.

## Tool Coordination

- **Bash**: git operations (fetch, diff, log) on the temp clone
- **Read / Grep**: navigate Go source to find the right place for each change
- **Edit**: apply targeted changes to existing Go files
- **Write**: update `.claude-agent-sdk-python-sha` with the new HEAD SHA

## Boundaries

**Will:**
- Backport protocol, options, and client behaviour changes from Python → Go
- Update `.claude-agent-sdk-python-sha` to the new HEAD after a successful backport
- Report clearly what was backported and what was intentionally skipped

**Will Not:**
- Blindly translate Python code — always adapt to Go idioms and project conventions
- Modify CI, packaging, or documentation files unless explicitly asked
- Force-push or amend commits
