# Roadmap

Ordered by how much each entry limits real deployments, not by how interesting it is to build.
Each entry says what breaks today, so it can be judged on its own.

## Closing the gap to `claude-agent-sdk-python`

The port is pinned to `566e41f` (2026-03-30); upstream is **447 commits ahead** and released
`v0.2.156` on 2026-09-18. The test suite is green and stays green, because no test can fail for a
feature that was never ported.

A single 447-commit catch-up is the change nobody dares review, so this is taken in slices,
each of which ships something usable on its own. The `UPSTREAM.md` pin moves only as far as a
slice actually verifies — a pin that jumps to HEAD because the tests passed is the same lie in a
newer commit.

The ordering principle is **live breaks before missing features**: a wire type the SDK gets wrong
corrupts what a working user already receives, while a feature that was never ported merely stays
absent. The first slice (content blocks) shipped on 2026-09-20 and found exactly that — a block
type the SDK had invented, dropping every server-side tool result. Expect the same shape in the
slices below: check what the CLI binary actually emits, not what the SDK expects.

### 1. Typed lifecycle and hook-event messages

**Today:** `task_started`, `task_progress`, `task_notification`, `task_updated`, `hook_started`
and `hook_response` all arrive as a generic `SystemMessage` with a `Subtype` string and a
`json.RawMessage`. Upstream types each of them. `task_updated` matters most: it is sometimes the
*only* notice that a task reached a terminal state, so a caller tracking subagent tasks in Go has
to hand-decode `Data` and know that `patch.status` is where terminal-ness lives.

**Why it is not simply done:** the subtype set grows with the CLI, so typing them must not turn an
unrecognised subtype into a dropped message — the same failure the content-block slice just fixed,
one level up. The generic `SystemMessage` has to stay as the fallback, and `Message` gains fields
rather than changing the existing one.

**Shape:** `Message.TaskUpdated`, `Message.HookEvent` and friends, populated from the `system`
subtypes; every unrecognised subtype keeps arriving as `System`. End to end against the stub CLI
with a recorded `task_updated` patch, and against a real `claude` run that spawns a subagent —
that one is provokable, unlike a server-side tool call.

### 2. `deferred_tool_use` on the result message

**Today:** `ResultMessage` drops the `deferred_tool_use` field upstream added. A turn that ended
with a tool call deferred to the caller looks, in Go, like a turn that ended with nothing pending.

**Shape:** a `DeferredToolUse` struct (`ID`, `Name`, `Input`) on `ResultMessage`. Small; the work
is confirming against the CLI binary what actually populates it and when, rather than copying the
Python dataclass and assuming.

### 3. Options and CLI flags

**Today:** `domains/agent/options.go` maps the flags as of March 2026. An option upstream added
that this does not map is a feature a user cannot reach at all — `BuildCLIArgs` has no escape
hatch for an unmapped flag.

**Shape:** diff `BuildCLIArgs` against upstream's `_build_command` and against `claude --help`
from the pinned CLI. Two sources, because upstream's own list has been behind the CLI before. Add
the missing flags, and consider a raw pass-through so the next gap is not a hard block.

### 4. Client lifecycle and session semantics

**Today:** upstream grew `session_resume`, `session_import`, `session_summary`,
`transcript_mirror_batcher` and a `testing/session_store_conformance` harness — roughly 2,700 new
lines across `_internal/sessions*.py`. This SDK's `domains/sessions` predates all of it.

**Why it is not simply done:** this is behavioural parity, not type parity, so it needs the
conformance harness ported before the features are — otherwise "done" is unfalsifiable.

**Shape:** port `session_store_conformance` first and run this SDK's store against it. What fails
becomes the sub-entries. Re-measure the gap before planning further; the slices above will have
moved it.

## Check the SDK's wire vocabulary against the CLI binary, mechanically

**Today:** nothing compares the block types, message types and control subtypes this SDK matches
against the ones the CLI actually emits. That is how `server_tool_result` — a name no CLI has ever
sent — sat in the parser for six months with a test asserting it on both sides. The test proved
the parser agreed with itself.

The answer was available the whole time: the `claude` binary contains its own exhaustive switch
over every content block it recognises, and `strings` finds it in one command. One grep would have
caught the bug on the day it was written.

**Why it is not simply done:** grepping a minified bundle for a switch statement is brittle — the
shape changes between CLI releases, and a check that silently stops matching is worse than no
check, because it reports healthy while doing nothing. So it has to fail loudly when it can no
longer find the vocabulary, not just when the vocabulary disagrees.

**Shape:** a script that extracts the block-type switch from the installed CLI, diffs it against
the constants in `domains/protocol`, and reports three lists: emitted-but-unmatched,
matched-but-never-emitted, and agreed. Run it in the maintenance pass. It must exit non-zero if
the extraction itself finds nothing — the failure mode to design against is a green tick over a
pattern that stopped matching. Falsify it against a deliberately wrong constant before trusting
a clean run.

## Make `transport.Receive` honour the context it is given

**Today:** `Receive(ctx)` ignores `ctx` entirely and blocks in a pipe read. Cancelling the context
does not interrupt it. This caused the `Disconnect` deadlock fixed on 2026-09-20, which is worked
around by closing the transport before waiting for the read loop — correct, but it leaves a public
interface whose context parameter is decorative. Any other caller that expects cancellation to
work will hit the same wall, and a custom `transport.Transport` implementation has no way to know
the parameter is not honoured.

**Shape:** a reader goroutine per transport handing lines to a channel, with `Receive` selecting
on that channel and `ctx.Done()`. Changes the contract for every implementation, so the
`transport.Transport` doc comment has to say what a conforming `Receive` must do. Test that a
cancelled context unblocks `Receive` against a subprocess that never writes — the stub CLI can be
told to go silent.

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
