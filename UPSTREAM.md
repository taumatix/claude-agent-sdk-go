# Upstream

This library is a Go port of a Python SDK, and it drives a Node CLI at runtime. Both move
without us. This file says which versions it was built and checked against, so you can judge
how current it is before depending on it.

```yaml
- name: claude-agent-sdk-python
  kind: github-commit
  repo: anthropics/claude-agent-sdk-python
  sha: 566e41f7a59377885693082d0e8436d8964a0491
  checked: 2026-09-20
  note: the Python SDK this library is ported from; the port mirrors its public surface

- name: claude-code-cli
  kind: npm
  package: "@anthropic-ai/claude-code"
  version: unpinned
  checked: 2026-09-20
  note: >-
    spawned as a subprocess by domains/transport/subprocess; no version floor is
    enforced yet. The content-block vocabulary in domains/protocol was read out
    of the 2.1.220 binary on 2026-09-20.
```

## Where this stands today — read this before depending on the port

**The Python SDK pin is six months stale.** It was set on 2026-03-30 when the port was written
and never moved. Upstream is **447 commits ahead** of it and released `v0.2.156` on 2026-09-18.

The pin has not moved, and it should not: on 2026-09-20 the first slice of the catch-up shipped —
the content-block vocabulary in `domains/protocol`, verified against the `claude` 2.1.220 binary
and covered end to end. That is one slice of one area, not the 447 commits, so moving the pin
would advertise a currency this port does not have.

**Something here *was* broken, and the green suite said otherwise.** The SDK matched a content
block type, `server_tool_result`, that no CLI has ever emitted; every server-side tool result was
being dropped and replaced with an empty text block. It shipped in 0.2.0 as a *fix* for that very
problem, and is actually fixed in 0.3.0. The test covering it asserted the invented name on both sides, so it passed for as long
as the parser was consistently wrong. Green tests prove what was ported still agrees with itself —
not that it agrees with the CLI, and not that it is all of what upstream now offers.

Treat the feature surface as "the Python SDK as of 2026-03-30, plus the content-block fix of
2026-09-20". Closing the rest is tracked in [ROADMAP.md](ROADMAP.md), split into slices ordered
live-breaks-first.

**The CLI version floor is undeclared.** This library spawns `@anthropic-ai/claude-code` and
speaks its stdio protocol. It does not check the CLI's version, so an old CLI fails at runtime
with a protocol error rather than a clear message. Also on the roadmap.

## How this file is kept honest

`checked` is bumped on every maintenance pass whether or not anything drifted — an unrefreshed
date cannot be told apart from an unchecked one. Drift is reported by:

    /Users/taumatix/bootstrap/bin/check-upstream-drift.py <checkout>

The ` ```yaml ` block above is the single source of truth for these pins. Nothing else in the
repo may hold a second copy; the previous pin lived in a dotfile that no reader and no scheduled
job ever opened, which is how six months went by unnoticed.
