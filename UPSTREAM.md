# Upstream

This library is a Go port of a Python SDK, and it drives a Node CLI at runtime. Both move
without us. This file says which versions it was built and checked against, so you can judge
how current it is before depending on it.

```yaml
- name: claude-agent-sdk-python
  kind: github-commit
  repo: anthropics/claude-agent-sdk-python
  sha: 566e41f7a59377885693082d0e8436d8964a0491
  checked: 2026-10-03
  note: >-
    the Python SDK this library is ported from; the port mirrors its public
    surface. 476 commits behind as of 2026-10-03. The pin stays at the original
    sha: four areas have since been brought level with upstream HEAD and verified
    against the CLI (content blocks on 2026-09-20, `system` lifecycle and hook
    messages on 2026-09-25, session state and background tasks on 2026-09-27,
    and ending a Query on the CLI's "idle" on 2026-10-03), which is four areas
    and not 476 commits.

- name: claude-code-cli
  kind: npm
  package: "@anthropic-ai/claude-code"
  version: 2.1.288
  checked: 2026-10-03
  hold: >-
    the CLI is spawned, not bundled, so the user's installed version is the one that
    runs; the gap is measured, not an alarm. Floor is MinimumCLIVersion = 2.0.0
    (warning only); unmodelled content blocks degrade to messages.UnknownBlock and
    unmodelled `system` subtypes to messages.SystemMessage with Raw intact.
  note: >-
    spawned as a subprocess by domains/transport/subprocess. `version` is the
    newest binary whose wire vocabulary was diffed against domains/protocol.
    2.1.267 checked twice: content blocks on 2026-09-21 (all seven server tool
    result types agree, no `server_tool_result`), and `system` subtypes on
    2026-09-25 against the zod schemas the bundle ships plus a live run — the
    four task subtypes and three hook phases agree field for field, and no
    system subtype carries a `data` key. 2.1.283 checked on 2026-09-27: the
    session_state_changed and background_tasks_changed schemas plus the full
    live end-to-end suite (task lifecycle, session state, no `data` key) pass.
    2.1.288 checked on 2026-10-03, run from its npm package without installing
    it: it honours CLAUDE_CODE_SDK_READS_SESSION_STATE (2.1.283 does not; the
    string is absent from that binary), sends sdk_host_only frames with "idle"
    after the result, and passes the full live suite. So does 2.1.283.
```

## Where this stands today — read this before depending on the port

**The Python SDK pin is six months stale.** It was set on 2026-03-30 when the port was written
and never moved. Upstream is **476 commits ahead** of it and released `v0.2.163` on 2026-09-30.
The gap grew by 13 commits between 2026-09-27 and 2026-10-03, and by 28 since 2026-09-21. It grows
every week this port does not close a slice, which is the honest way to read this number.

The pin has not moved, and it should not. Four slices of the catch-up have shipped:

- **Content blocks** (2026-09-20, released as `v0.3.0`) — the vocabulary in `domains/protocol`,
  verified against the `claude` binary and covered end to end.
- **`system` lifecycle and hook messages** (2026-09-25) — the four `task_*` subtypes and the three
  hook phases, brought level with upstream HEAD and then past it: the CLI emits a `hook_progress`
  phase and five `task_started` fields that upstream's Python SDK does not model.
- **Session state and background tasks** (2026-09-27, `v0.5.0`) — `session_state_changed` and
  `background_tasks_changed`, typed from the 2.1.283 schemas. Upstream types neither.
- **When a run is over** (2026-10-03, `v0.6.0`) — a `Query` waits for the CLI's `idle`, requested
  through `CLAUDE_CODE_SDK_READS_SESSION_STATE` as upstream does, bounded by the same ceiling.
  Upstream's in-flight agent-task tracking, for CLIs that report no state, is not ported yet
  (ROADMAP entry 0b).

That is four areas, not 476 commits, so moving the pin would advertise a currency this port does
not have.

**Something here *was* broken, and the green suite said otherwise.** The SDK matched a content
block type, `server_tool_result`, that no CLI has ever emitted; every server-side tool result was
being dropped and replaced with an empty text block. 0.2.0's changelog claims to have fixed that
very problem; it did not, and 0.3.0 does. The test covering it asserted the invented name on both
sides, so it passed for as long as the parser was consistently wrong. Green tests prove what was
ported still agrees with itself — not that it agrees with the CLI, and not that it is all of what
upstream now offers.

**And something else was broken the same way.** `messages.SystemMessage.Data` was bound to a
`data` key that no `claude` release up to 2.1.267 emits — every `system` subtype puts its fields
at the top level. Upstream's Python `SystemMessage.data` is the *whole message dict*; the port
bound that name to a nested object that has never existed, so the payload of every system message
was unreachable from Go. Fixed on 2026-09-25 by adding `Raw` and deprecating `Data` in place.
Twice now the green suite has been green over a field this SDK invented.

Treat the feature surface as "the Python SDK as of 2026-03-30, plus the content-block fix of
2026-09-20 (`v0.3.0`, 2026-09-21), plus the typed `system` lifecycle and hook messages of
2026-09-25, plus typed session state and background tasks (`v0.5.0`, 2026-09-27)". Closing the rest is tracked in [ROADMAP.md](ROADMAP.md), split into slices ordered
live-breaks-first.

**The CLI version floor is soft.** This library spawns `@anthropic-ai/claude-code` and speaks its
stdio protocol. It reads `claude -v` at construction and logs a warning below
`subprocess.MinimumCLIVersion` (2.0.0), then carries on — so an old CLI still fails later with a
protocol or JSON decode error that points at this library rather than at the real cause. Turning
that warning into a refusal with a clear message is on the roadmap.

## How this file is kept honest

`checked` is bumped on every maintenance pass whether or not anything drifted — an unrefreshed
date cannot be told apart from an unchecked one. Drift is reported by:

    /Users/taumatix/bootstrap/bin/check-upstream-drift.py <checkout>

The ` ```yaml ` block above is the single source of truth for these pins. Nothing else in the
repo may hold a second copy; the previous pin lived in a dotfile that no reader and no scheduled
job ever opened, which is how six months went by unnoticed.
