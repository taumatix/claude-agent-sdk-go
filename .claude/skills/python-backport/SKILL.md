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

- **[UPSTREAM.md](../../../UPSTREAM.md) at the repo root is the single source of truth** for the
  last Python SDK commit that was backported — the `claude-agent-sdk-python` pin in its fenced
  ` ```yaml ` block. It replaced a `.claude-agent-sdk-python-sha` dotfile, which nothing but this
  skill ever read: the pin went six months and 446 commits stale without anyone noticing, because
  a version a reader cannot see is a version nobody checks.
- The upstream Python SDK lives at https://github.com/anthropics/claude-agent-sdk-python.git
- This Go SDK mirrors the Python SDK's architecture and protocol semantics.
- To see the current gap without cloning anything:
  `/Users/taumatix/bootstrap/bin/check-upstream-drift.py <checkout>`

## Behavioral Flow

1. **Read current SHA**: Read the `sha:` of the `claude-agent-sdk-python` pin in `UPSTREAM.md`.

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

6. **Update the pin**: Write the new HEAD SHA into the `claude-agent-sdk-python` pin in
   `UPSTREAM.md` and bump its `checked:` date. This must happen regardless of whether any
   backport changes were made — if the SDKs are already in sync, the pin still moves, because an
   unrefreshed date cannot be told apart from an unchecked one. Update the README's upstream note
   and the "where this stands today" section of `UPSTREAM.md` in the same commit, or the repo
   goes back to advertising a staleness it no longer has.

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
- **Edit**: move the `claude-agent-sdk-python` pin in `UPSTREAM.md` to the new HEAD SHA

## Boundaries

**Will:**
- Backport protocol, options, and client behaviour changes from Python → Go
- Move the `UPSTREAM.md` pin to the new HEAD after a successful backport, and bring the README's
  upstream note with it
- Report clearly what was backported and what was intentionally skipped

**Will Not:**
- Blindly translate Python code — always adapt to Go idioms and project conventions
- Modify CI, packaging, or documentation files unless explicitly asked
- Force-push or amend commits
