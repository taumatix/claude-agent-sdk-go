# Upstream

This library is a Go port of a Python SDK, and it drives a Node CLI at runtime. Both move
without us. This file says which versions it was built and checked against, so you can judge
how current it is before depending on it.

```yaml
- name: claude-agent-sdk-python
  kind: github-commit
  repo: anthropics/claude-agent-sdk-python
  sha: 566e41f7a59377885693082d0e8436d8964a0491
  checked: 2026-09-19
  note: the Python SDK this library is ported from; the port mirrors its public surface

- name: claude-code-cli
  kind: npm
  package: "@anthropic-ai/claude-code"
  version: unpinned
  checked: 2026-09-19
  note: spawned as a subprocess by domains/transport/subprocess; no version floor is enforced yet
```

## Where this stands today — read this before depending on the port

**The Python SDK pin is six months stale.** It was set on 2026-03-30 when the port was written
and never moved. Upstream is **446 commits ahead** of it and released `v0.2.156` on 2026-09-18.

Nothing here is known to be broken — the test suite is green and the CLI protocol this library
speaks has been stable — but **green tests prove only that what was ported still works, not that
it is all of what upstream now offers.** Anything added to `claude-agent-sdk-python` since March
2026 is missing here, and no test can fail for a feature that was never written. Treat the
feature surface as "the Python SDK as of 2026-03-30" until this pin moves.

Closing that gap is tracked in [ROADMAP.md](ROADMAP.md), split into reviewable pieces. A single
446-commit catch-up is exactly the change nobody dares to review, so it is being taken in order
of what actually limits users.

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
