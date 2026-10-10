# Roadmap

Ordered by how much each entry limits real deployments, not by how interesting it is to build.
Each entry says what breaks today, so it can be judged on its own.

## Closing the gap to `claude-agent-sdk-python`

The port is pinned to `566e41f` (2026-03-30); upstream is **476 commits ahead** as of 2026-10-03
and released `v0.2.163` on 2026-09-30. The test suite is green and stays green, because no test
can fail for a feature that was never ported.

A single 476-commit catch-up is the change nobody dares review, so this is taken in slices,
each of which ships something usable on its own. The `UPSTREAM.md` pin moves only as far as a
slice actually verifies — a pin that jumps to HEAD because the tests passed is the same lie in a
newer commit.

The ordering principle is **live breaks before missing features**: a wire type the SDK gets wrong
corrupts what a working user already receives, while a feature that was never ported merely stays
absent. Four slices have shipped. The first two found a live break rather than a missing
feature; the third (session state and background tasks, 2026-09-27) found one next to it, and the
fourth (2026-10-03) fixed it for CLIs that report state. A fifth (2026-10-09) closed the
ledger-reset question: a ledger lives and dies with its `sessionManager`, so every `Connect` starts
empty, and a repeated `initialize` cannot occur because this SDK initializes once per process.
Entry 0b remains:

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

### 0b. `CLIPath` pointed at an unreleased CLI is the only way to test a newer one

The 0.6.0 change was verified against 2.1.288 by downloading its npm package and pointing
`CLAUDE_SDK_LIVE_CLI_PATH` at the binary, because the installed CLI is 2.1.283 and upgrading it is
not this repo's to do. That worked once, by hand. A maintenance pass should do the same against
the newest published CLI every time, because the behaviour this SDK depends on (which env
variables are honoured, where `idle` falls) changes between CLI releases with nothing in this repo
noticing.

### 1. The rest of the `system` subtype vocabulary

**Today:** the task and hook subtypes are typed (2026-09-25), and `background_tasks_changed` and
`session_state_changed` (0.5.0, 2026-09-27). The CLI's schema bundle declares many more that
still arrive as a generic `System`: `init`, `status`, `thinking_tokens`, `task_summary`,
`post_turn_summary`, `compact_boundary`, `files_persisted`, `file_snapshot`, `mirror_error`,
`code_change_published`, `vcs_state_changed`, `commands_changed`, `elicitation_complete`,
`plugin_install`, `local_command_output`, `informational`, `feedback_draft_queued`,
`worker_shutting_down`, `auth_status`, `turn_duration`, `dev_intent`, `session_metadata`.

`permission_denied` (0.8.0) and `api_retry` (0.9.0) are typed: the two a caller would act on
rather than display. `thinking_tokens` (0.10.0) is typed too; a reasoning prompt produces it
live. `task_summary` and `post_turn_summary` are **not** to be typed: the 2.1.288 schema marks
both `@internal`, so their fields carry no promise, and a typed API over them would make one this
SDK cannot keep. They arrive as `System`, with `Raw`. `compact_boundary` (0.11.0) is typed with
only its public fields (`trigger`, token counts, duration); `/compact` after one turn produces a
real frame for about $0.1. The one trigger value seen live is `manual`; an automatic compaction
has not been captured, so its `trigger` value is unverified. `informational` (`content`, `level`, `prevent_continuation`) is typed too, from the schema
only: no live CLI has been seen to send one, so its `level` values are unknown and the next pass over this entry
should try to provoke one. The next candidate is `status`. For anything about API failures, a
local server answering 529 through `ANTHROPIC_BASE_URL` produces real frames without credentials.

**Why it is not simply done:** that is 20+ subtypes and typing all of them in one change is the
review nobody wants. Rank by whether a Go caller can act on it; the `@internal`-marked ones
probably never.

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

### 4. `deferred_tool_use` is unverified against a live CLI

**Today:** `ModelUsage` was confirmed broken against 2.1.283 and fixed (2026-10-10: the CLI sends `modelUsage`;
every result frame has it). `DeferredToolUse` (2026-10-10) came from upstream's parser and has not been seen live.
Several other `ResultMessage` keys the SDK reads were not compared against a live frame either; a live 2.1.283
result carries `result_index`, `subagent_stats`, `fast_mode_state`, `queued_turn_count`, `ttft_ms` and
`first_content_frame_ms`, which the SDK does not model.

**Shape:** make a PreToolUse hook answer `"defer"` against a live run to see the key; decide which of the unmodelled
result keys a caller would act on (`subagent_stats`, `terminal_reason` is done) and type those.

### 5. Options and CLI flags

**Today:** `BuildCLIArgs` maps the flags as of March 2026 plus five added 2026-10-09 (`--json-schema`,
`--no-session-persistence`, `--strict-mcp-config`, `--plugin-dir`, `--include-hook-events`), then `--agents`, `--disable-slash-commands`, `--plugin-url`, `--worktree`, `--from-pr`, `--permission-prompt-tool` and `--permission-prompts` (the last four 2026-10-09). `--permission-prompt-tool` is typed but nothing checks how it interacts with the SDK's own `can_use_tool` control path. `ExtraArgs`
is the escape hatch, but it is a map, so a repeatable flag or an ordering needs a typed option.

**Upstream diff** (2026-10-10, against `_build_command` on main): upstream's flags missing here were
`--thinking`, `--thinking-display` and `--system-prompt-file` (typed since), `--task-budget` and `--session-mirror`.
`--task-budget` is accepted by 2.1.283 but a small value (5000) ended a turn at once with no cost, so its
semantics (a token budget for the task? a minimum?) need a look before it is typed; `--session-mirror` belongs to
entry 6b.

**Left:** `claude --help` (2.1.283) lists more with no typed option: `--bare` (it changes how the CLI
authenticates), `--replay-user-messages` (adds frames to the stream), `--tmux` (needs `--worktree`; interactive),
`--ide` (interactive), and the fields of `--agents` beyond `description` and `prompt` (tools, model:
unverified). `--brief` is typed (`Options.Brief`, 2026-10-09); what its SendUserMessage tool does to the
stream is unobserved. `ResultMessage.StructuredOutput` already carries `--json-schema`'s answer (parser
test only; no live run). Each remaining flag needs a look at what the SDK does with the new frames before
it is exposed.
Also diff against upstream's `_build_command`, since upstream's list has been behind the CLI before.

### 6. Client lifecycle and session semantics, after the conformance harness

**Done:** `sessions.SessionStore` (`Append`/`Load`, with optional `SessionLister`, `SessionDeleter`,
`SubkeyLister`), `MemorySessionStore`, and `sessionstoretest` — contracts 1 to 13 of upstream's
`session_store_conformance`. The older `sessions.Store` is untouched.

**Left, in order** (each ships something usable):

- **6a. Run a real adapter against the store contracts:** `ListSessionsFromStore` shipped (2026-10-10) with
  `MemorySessionStore` and counting stores as its only tests; no adapter outside this repo (Postgres, S3, Redis) has
  been run against contract 14 or this listing, so the freshness rule (summary mtime >= listed mtime) is unproven on a
  store whose two clocks differ. Add a reference adapter over a real backend, or a conformance case that injects clock
  skew.
- **6b. Transcript mirroring:** `transcript_mirror` frames from the CLI appended to a configured
  `SessionStore` (`Options.SessionStore`), batched and eager flush modes, `mirror_error` surfaced when
  an `Append` fails. Needs an e2e against the fake CLI emitting those frames.
- **6c. Store-backed resume:** `Resume` materialising a session from the store when the local
  transcript is absent (`session_resume`), and `session_import`.

The frame names and flush semantics are from upstream's source, not a live CLI; verify 6b against one.

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

## A `Send` cut off part-way still poisons the session

**Today:** a cancelled `Send` that landed no byte, or never started, leaves the transport usable
(2026-10-09). One that was cut off after some bytes closes stdin and every later `Send` fails, because
the CLI has half a JSON line and the next message would be glued to it. Only lines over the pipe buffer
(64 KiB on Linux, 16-64 KiB on macOS) can end up there, which in practice means a large attachment.

**Shape:** the stream could be repaired rather than closed: after a partial write, finish the line
with a `\n` the CLI will reject as malformed, and see what the CLI does with it. Needs the live CLI to
show whether it survives a bad line; until then closing is the safe answer. Also unchecked: Windows,
where pipes have no write deadline and every cancel mid-write still closes stdin.

## Make the `claude` CLI version floor the default

**Today:** `Options.RequireMinimumCLIVersion` refuses a CLI below 2.0.0 with `*errors.CLIVersionError`
(2026-10-09), but it is opt-in, so a library consumer who does not know of it still gets the log line
and a later protocol error. The floor itself (2.0.0) is from the original port; nothing has measured
what the oldest CLI this SDK really works with is.

**Shape:** announce in a release's notes, then flip the default in the next minor with
`Options.SkipVersionCheck` as the opt-out; first run the live test against the oldest CLI still on npm
that speaks the stdio protocol and raise `MinimumCLIVersion` to what it shows.

## What the surface inventory found missing

**Today:** `docs/upstream-surface.txt` accounts for all 159 names upstream exports (2026-10-10): 65 ported, 48
partial (a string or raw JSON where upstream has a type), 44 missing, 2 skipped. Missing is not evenly spread. Ordered
by what limits a deployment:

1. **In-process SDK MCP servers** (`create_sdk_mcp_server`, `tool`, `McpSdkServerConfig`, `SdkMcpTool`,
   `ToolAnnotations`): a Go caller cannot expose a function as a tool without running a separate MCP process.
2. **The typed `system/init` message**, and the remaining control requests upstream's client sends (`rewind_files`,
   `stop_task`, and others in `_internal/query.py`). `MCPStatus`, `ContextUsage`, `ReconnectMCPServer` and
   `ToggleMCPServer` are verified against the stub CLI only, whose bodies and request shapes (`mcp_reconnect`,
   `mcp_toggle`) are read from the Python SDK, not captured from a live CLI.
3. **Store-backed session functions** (`fork_session_via_store` and subagent listing remain; the info and message reads
   shipped 2026-10-10 and rename, tag and delete the same day): a store can be listed, read, renamed, tagged and deleted
   from, but not forked. `TagSessionViaStore` skips upstream's NFKC step (it needs `golang.org/x/text`, a first
   dependency); add it if a tag with compatibility characters matters. None of the three has run against a store but
   `MemorySessionStore`.
4. **Sandbox settings, `TaskBudget`, typed hook inputs and outputs**, and `CanUseToolShadowedWarning`.

**Done:** permission results (`Options.ToolPermissionFunc`, 2026-10-10). Not yet seen against a live CLI: whether a
live `can_use_tool` request carries the optional context fields, and whether `updatedPermissions` from a suggestion is
accepted. A live run that makes a Bash call under `default` permission mode would show both.

**Shape:** each of 1-3 is its own entry when it reaches the top; do not ship them as one.

Left over from the CI wiring: the `Upstream surface` workflow (2026-10-10, weekly and on changes to the inventory) fails when upstream
exports a name the inventory does not account for, but nothing makes a failed scheduled run reach a pass: the
maintenance pass should read it, and `check-repo-health.py` counts only the `CI` workflow.
