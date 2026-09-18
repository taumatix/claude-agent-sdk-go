# Roadmap

Ordered by how much each entry limits real deployments, not by how interesting it is to build.
Each entry says what breaks today, so it can be judged on its own.

## Close the six-month gap to `claude-agent-sdk-python`

**Today:** the port is pinned to `566e41f` (2026-03-30) and upstream is 446 commits ahead, having
released `v0.2.156` on 2026-09-18. The test suite is green and stays green, because no test can
fail for a feature that was never ported. Users get the Python SDK's surface as it stood in March
2026 and no way to tell from the code that this is so — the README now says it, which is the
first half of the fix.

**Why it is not simply done:** a single 446-commit catch-up is the change nobody dares review,
and reviewing it is the only thing standing between a port and a plausible-looking guess. It also
cannot be judged as one unit: some of those commits are protocol changes this SDK must match to
keep working, and some are Python packaging noise that must not be translated at all.

**Shape:** take it in slices, newest-breaking-first rather than chronologically. Diff the two
public surfaces — not the two diffs — to get the real gap, then order it:

1. **Protocol and wire types** (`domains/protocol`) — the CLI and this SDK must agree, so drift
   here is a live break, not a missing feature. Highest priority and testable end to end against
   a real `claude` binary.
2. **Options and CLI flags** (`domains/agent/options.go`) — an option upstream added that this
   does not map is a feature a user cannot reach.
3. **Client lifecycle and session semantics** — behavioural parity, needs the harness.
4. **Everything else**, as separate entries once the above is landed and the gap is re-measured.

Move the `UPSTREAM.md` pin only as far as each slice actually verifies. A pin that jumps to HEAD
because the tests passed is the same lie in a newer commit.

## Declare and enforce the `claude` CLI version floor

**Today:** the SDK spawns `@anthropic-ai/claude-code` (latest 2.1.277) and speaks its stdio
protocol, with no version check. An old CLI fails at runtime with a protocol or JSON decode error
that points at this library rather than at the real cause, and `UPSTREAM.md` has to record the
floor as `unpinned` because there is nothing to record.

**Shape:** run `claude --version` during the subprocess handshake, compare against a declared
minimum, and fail with a message naming the CLI and the required version. The floor becomes a
real pin in `UPSTREAM.md`. The end-to-end test needs a real CLI on PATH plus a stub that reports
an old version.

## Surface parity checked by a test, not by a pass

**Today:** whether this SDK has fallen behind is answered by a human reading two codebases, which
is why the answer went six months out of date. `check-upstream-drift.py` now reports the commit
gap, but a commit count says nothing about which *symbols* are missing.

**Shape:** a generated inventory of the Python SDK's public surface, committed here, and a test
that fails when the live upstream has a public symbol this inventory does not explain — either
"ported" or "deliberately skipped, because". Deliberate omissions are fine; undiscovered ones
are the problem.
