# Upstream

This library is a Go port of a Python SDK, and it drives a Node CLI at runtime. Both move
without us. This file says which versions it was built and checked against, so you can judge
how current it is before depending on it.

```yaml
- name: claude-agent-sdk-python
  kind: github-commit
  repo: anthropics/claude-agent-sdk-python
  sha: 566e41f7a59377885693082d0e8436d8964a0491
  checked: 2026-09-24
  note: the Python SDK this library is ported from; the port mirrors its public surface

- name: claude-code-cli
  kind: npm
  package: "@anthropic-ai/claude-code"
  version: 2.1.267
  checked: 2026-09-24
  hold: >-
    the CLI is spawned, not bundled, so the user's installed version is the one that
    runs; the gap is measured, not an alarm. Floor is MinimumCLIVersion = 2.0.0
    (warning only) and unmodelled blocks now degrade to messages.UnknownBlock.
  note: >-
    spawned as a subprocess by domains/transport/subprocess. `version` is the
    newest binary whose content-block vocabulary was diffed against
    domains/protocol — 2.1.267 on 2026-09-21, agreeing on all seven server tool
    result types and carrying no `server_tool_result`.
```

## Where this stands today — read this before depending on the port

**The Python SDK pin is six months stale.** It was set on 2026-03-30 when the port was written
and never moved. Upstream is **456 commits ahead** of it and released `v0.2.159` on 2026-09-23.
The gap has grown by 8 commits since 2026-09-21; it grows every week this port does not close a
slice, which is the honest way to read this number.

The pin has not moved, and it should not: on 2026-09-20 the first slice of the catch-up shipped —
the content-block vocabulary in `domains/protocol`, verified against the `claude` 2.1.220 binary
and covered end to end, released as `v0.3.0`. That is one slice of one area, not the 456 commits,
so moving the pin would advertise a currency this port does not have.

**Something here *was* broken, and the green suite said otherwise.** The SDK matched a content
block type, `server_tool_result`, that no CLI has ever emitted; every server-side tool result was
being dropped and replaced with an empty text block. 0.2.0's changelog claims to have fixed that
very problem; it did not, and 0.3.0 does. The test covering it asserted the invented name on both
sides, so it passed for as long as the parser was consistently wrong. Green tests prove what was
ported still agrees with itself — not that it agrees with the CLI, and not that it is all of what
upstream now offers.

Treat the feature surface as "the Python SDK as of 2026-03-30, plus the content-block fix of
2026-09-20, released as `v0.3.0` on 2026-09-21". Closing the rest is tracked in
[ROADMAP.md](ROADMAP.md), split into slices ordered live-breaks-first.

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
