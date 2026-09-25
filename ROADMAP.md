# Roadmap

Ordered by how much each entry limits real deployments, not by how interesting it is to build.
Each entry says what breaks today, so it can be judged on its own.

## Closing the gap to `claude-agent-sdk-python`

The port is pinned to `566e41f` (2026-03-30); upstream is **459 commits ahead** as of 2026-09-25
and released `v0.2.159` on 2026-09-23. The test suite is green and stays green, because no test
can fail for a feature that was never ported.

A single 459-commit catch-up is the change nobody dares review, so this is taken in slices,
each of which ships something usable on its own. The `UPSTREAM.md` pin moves only as far as a
slice actually verifies — a pin that jumps to HEAD because the tests passed is the same lie in a
newer commit.

The ordering principle is **live breaks before missing features**: a wire type the SDK gets wrong
corrupts what a working user already receives, while a feature that was never ported merely stays
absent. Two slices have shipped and both found a live break rather than a missing feature:

- **Content blocks** (2026-09-20) — the SDK matched `server_tool_result`, a name no CLI emits, and
  was dropping every server-side tool result.
- **System lifecycle messages** (2026-09-25) — `SystemMessage.Data` was bound to a `data` key that
  no CLI emits, so the payload of *every* system message was unreachable. The entry had this filed
  as "the caller has to hand-decode `Data`"; there was nothing in `Data` to decode.

Both were found the same way and it is the instruction for every slice below: **check what the CLI
binary actually emits, not what the SDK expects, and not what upstream's dataclasses say.** The
binary is a usable reference — it ships zod schemas naming every field of every message it sends
(see entry 2). Reading them also found `hook_progress`, a message upstream's Python SDK does not
model at all.

### 1. The rest of the `system` subtype vocabulary

**Today:** the task and hook subtypes are typed (shipped 2026-09-25). The CLI's schema bundle
declares many more that still arrive as a generic `System`, and two of them have a caller waiting:
`background_tasks_changed` is a *level* signal listing every live background task, and its own
schema says consumers who only need "is background work running" should replace their set from it
rather than pairing `task_started`/`task_notification` edges — which is exactly what the shipped
slice makes a caller do, so a missed bookend can still wedge a stale indicator.
`session_state_changed` (`idle`/`running`/`requires_action`) is described in the bundle as the
"authoritative turn-over signal".

Others seen or declared: `init`, `status`, `thinking_tokens`, `task_summary`, `post_turn_summary`,
`compact_boundary`, `files_persisted`, `file_snapshot`, `mirror_error`, `code_change_published`,
`vcs_state_changed`, `commands_changed`, `elicitation_complete`, `plugin_install`,
`local_command_output`, `informational`, `feedback_draft_queued`, `worker_shutting_down`,
`auth_status`, `turn_duration`, `dev_intent`.

**Why it is not simply done:** that is 20+ subtypes and typing all of them in one change is the
review nobody wants. Rank by whether a Go caller can act on it: `background_tasks_changed` and
`session_state_changed` first, the `@internal`-marked ones probably never.

**Shape:** one sub-entry per subtype worth typing, same pattern as the lifecycle slice — payload
struct in `protocol`, public type in `messages`, `System` still populated, unmodelled subtypes
still fall through. `mirror_error` is the odd one: upstream synthesises it in the SDK rather than
receiving it from the CLI, so it only makes sense once session mirroring exists here (entry 5).

### 2. Extract the system-subtype vocabulary from the CLI, mechanically

**Today:** the `claude` bundle turns out to ship **zod schemas for every `system` subtype it
emits**, naming each field and its optionality — `c({type:k("system"),subtype:k("task_updated"),
task_id:s(),patch:c({status:X([...])...})})`. The lifecycle slice was built by reading them out
of the binary by hand.

This is a much better source than the entry below assumed, and it is worth saying why: a grep for
block *names* cannot tell a wire type from a telemetry event, but a schema says what the fields
are. It is also how the slice found that `hook_progress` exists and that **no system subtype has a
`data` key** — the field the Go port had bound `SystemMessage.Data` to since March.

**Why it is not simply done:** same brittleness as the content-block check below. The minifier's
variable names change between releases, so the extractor has to key off the stable
`type:k("system"),subtype:k("...")` shape and **fail loudly when it matches nothing**.

**Shape:** merge with the content-block check below into one `bin/check-cli-vocabulary.py` that
reports, for both content blocks and system subtypes: emitted-but-unmatched,
matched-but-never-emitted, and field-level drift against the structs in `domains/protocol`. Run it
in the maintenance pass. Falsify it against a deliberately wrong constant before trusting a clean
run.

### 3. Make the live end-to-end path runnable by something other than me

**Today:** `domains/agent/live_e2e_test.go` drives the real `claude` binary and is the only test
that can catch the SDK believing a wire shape the CLI does not send — the failure that hid a
broken parser for six months. It is gated behind `CLAUDE_SDK_LIVE_E2E=1` because it needs
credentials and spends money, so **CI never runs it** and it fires only when a maintenance or
roadmap pass happens to run it by hand. A regression between passes is invisible.

**Why it is not simply done:** it needs a credential CI does not have, and the repo has no secret
set. The same gap is open on `anthropic-swift` (#5) and is a decision for my human, not for me.

**Shape:** either a repository secret and a scheduled (not per-PR) workflow that runs the live
tests and reports cost, or — cheaper and with no credential at all — record one real session's
frames to a golden file, replay it through the transport in CI, and have the maintenance pass
re-record and diff. The recording catches wire drift; it does not catch a CLI that stops speaking
to us at all.

### 4. `deferred_tool_use` on the result message

**Today:** `ResultMessage` drops the `deferred_tool_use` field upstream added. A turn that ended
with a tool call deferred to the caller looks, in Go, like a turn that ended with nothing pending.

**Shape:** a `DeferredToolUse` struct (`ID`, `Name`, `Input`) on `ResultMessage`. Small; the work
is confirming against the CLI binary what actually populates it and when, rather than copying the
Python dataclass and assuming.

### 5. Options and CLI flags

**Today:** `domains/agent/options.go` maps the flags as of March 2026. An option upstream added
that this does not map is a feature a user cannot reach at all — `BuildCLIArgs` has no escape
hatch for an unmapped flag.

**Shape:** diff `BuildCLIArgs` against upstream's `_build_command` and against `claude --help`
from the pinned CLI. Two sources, because upstream's own list has been behind the CLI before. Add
the missing flags, and consider a raw pass-through so the next gap is not a hard block.

### 6. Client lifecycle and session semantics

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

A coarse version of this was run by hand on 2026-09-21 against the 2.1.267 binary:
`strings | grep -oE '[a-z_]+_tool_result'` returned 14 names, agreeing with all seven the SDK
models and carrying no `server_tool_result`. It also returned `tengu_advisor_tool_result`,
`tengu_unexpected_tool_result` and `delivered_as_tool_result`, which are telemetry event names
rather than wire types — **so a name-shaped grep over the whole bundle cannot tell a content block
from an analytics event**, and the real check has to scope itself to the switch, not to the file.

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

**Today:** the SDK spawns `@anthropic-ai/claude-code` (latest 2.1.278) and speaks its stdio
protocol. It *does* check the version — `checkVersion` has run `claude -v` since the original port
and logs a warning below `MinimumCLIVersion` (2.0.0) — but the check is non-fatal and writes to
`log.Printf`, so a library consumer with a structured logger never sees it. The run then continues
and fails later with a protocol or JSON decode error that points at this library rather than at the
real cause. Until 2026-09-21 both this entry and `UPSTREAM.md` claimed there was no check at all.

**Shape:** make the floor a refusal, not a warning — a typed error from `domains/errors` naming the
CLI, the version found and the version required, returned from transport construction. That is a
behaviour change for anyone running an ancient CLI *successfully*, so it needs an opt-out
(`Config.SkipVersionCheck`) rather than a silent break. The end-to-end test needs a real CLI on
PATH plus a stub that reports an old version.

## Surface parity checked by a test, not by a pass

**Today:** whether this SDK has fallen behind is answered by a human reading two codebases, which
is why the answer went six months out of date. `check-upstream-drift.py` now reports the commit
gap, but a commit count says nothing about which *symbols* are missing.

**Shape:** a generated inventory of the Python SDK's public surface, committed here, and a test
that fails when the live upstream has a public symbol this inventory does not explain — either
"ported" or "deliberately skipped, because". Deliberate omissions are fine; undiscovered ones
are the problem.
