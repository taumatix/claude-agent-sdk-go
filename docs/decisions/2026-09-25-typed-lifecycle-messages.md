# Typed lifecycle and hook-event messages

**Goal**: give Go callers typed `task_*` and `hook_*` system messages, so tracking a subagent's
terminal state does not require hand-decoding a field that is not there.

## What we found before writing anything

The roadmap entry said a caller "has to hand-decode `Data`". That premise was wrong in the same
direction as the `server_tool_result` bug it was written after: **`Data` is empty.**

`protocol.SystemMessage` binds `Data` to `json:"data"`. No `system` message the CLI emits carries a
`data` key. Two independent checks:

- The `claude` 2.1.267 bundle ships zod schemas for every system subtype it emits. None of them
  declares `data`. (`grep -o 'subtype:k("[a-z_]*"),data:'` over the bundle's strings: no matches.)
- A live `claude -p ... --output-format stream-json` run on 2026-09-25 that spawned an `Explore`
  subagent produced 24 frames covering `hook_started`, `hook_response`, `init`, `thinking_tokens`,
  `task_started`, `task_progress`, `task_updated` and `task_notification`. Not one carried `data`.

Upstream Python's `SystemMessage.data` is the **entire message dict** (`_internal/message_parser.py`
passes `data=data`). The Go port bound that name to a nested object that has never existed, so every
`SystemMessage` handed to a Go caller since March 2026 has had `Data == nil` and no way to reach the
payload at all. That is a live break, not a missing feature, so it is fixed in this slice.

## Approach chosen

Wire structs in `domains/protocol` (json tags, mirroring the CLI's own schemas), public types in
`domains/messages`, converted in `FromWire` — the split the package already uses.

Three decisions worth stating:

1. **`Message.System` stays populated for typed subtypes.** Upstream models these as *subclasses* of
   `SystemMessage` precisely so `isinstance(msg, SystemMessage)` keeps matching. Go has no
   subclassing, so the equivalent is to set both `Message.System` and the new typed field. A caller
   switching on `msg.System != nil` today keeps working unchanged. This relaxes `Message`'s
   "exactly one non-nil field" invariant, which is a documentation change, not an API break.

2. **A malformed lifecycle event degrades to `System`; it never fails the stream.** Upstream raises
   `MessageParseError` on a missing key in `task_started`, which lets a progress notification kill a
   run. Upstream's own `task_updated` branch says parsing "must never raise on a lifecycle event";
   we apply that rule to all seven subtypes. An unrecognised subtype likewise keeps arriving as
   `System` — the same forward-compatibility rule the content-block slice established.

3. **`SystemMessage.Data` is deprecated, not repurposed.** Rebinding it to the whole message would
   match upstream, but it would silently change the meaning of a shipped field. A new `Raw` field
   carries the complete message; `Data` keeps its (never-populated) binding and a doc comment saying
   so. Backward compatibility is the product.

## What the CLI models that upstream Python does not

Read off the bundled schemas, not inferred:

- **`hook_progress` is a third hook phase.** Upstream's parser routes only `hook_started` and
  `hook_response`; the CLI emits `hook_progress` from a ~1s interval timer while a hook runs, with
  `stdout`/`stderr`/`output`. Porting upstream verbatim would have inherited the gap.
- `hook_started`/`hook_progress`/`hook_response` carry **both** `hook_name` (e.g.
  `"SessionStart:startup"`) and `hook_event` (e.g. `"SessionStart"`). Upstream collapses them into
  one field via `data.get("hook_event") or data.get("hook_name") or ...`; they are different things
  and are modelled separately here.
- `task_started` carries `subagent_type`, `is_backgrounded`, `spawn_depth`, `workflow_name`,
  `prompt`, `skip_transcript` and `ambient`; upstream models only `task_type` and `tool_use_id`.
  The live capture populated `subagent_type`, `is_backgrounded`, `spawn_depth`, `task_type` and
  `prompt`.
- `task_progress` carries `subagent_type` and `summary`; upstream models neither.
- `task_notification` carries `resource_links`, `skip_transcript` and `ambient`; upstream models
  none of the three. `resource_links` is left raw — its element schema is a separate slice.

## Alternatives considered

- **Decode lazily, via a `msg.System.As(&task)` helper.** Rejected: it keeps the cost of knowing
  which subtypes exist on the caller, which is the thing the entry exists to remove, and it gives
  the compiler nothing to check.
- **One `TaskMessage` with a `Subtype` discriminator and a union of every field.** Rejected: the
  four task subtypes barely overlap (`patch` only on `task_updated`, `usage` required on
  `task_progress` and optional on `task_notification`), so a merged struct would be mostly nil and
  would not say which fields are valid together.
- **Port upstream's shapes verbatim for fidelity.** Rejected on the doctrine the last slice
  established: check what the CLI emits, not what the SDK expects. Doing so would have missed
  `hook_progress` entirely.

## Libraries added or used

None. `encoding/json` and the existing `testify` assertions only.

## Trade-offs and risks

- **Optionality is modelled unevenly, on purpose.** `*bool` where absence means "not applicable"
  (`is_backgrounded`: the schema says it is set only for `local_agent`/`local_bash` tasks); plain
  `bool` where the schema's own description defines absence as false (`ambient`, `skip_transcript`).
- **`TaskStatus.IsTerminal` spans two vocabularies.** `task_notification` reports `stopped`;
  `task_updated` reports the raw `killed` for the same transition. Both are terminal, and the
  helper exists so a caller does not have to know that.
- **Only `local_agent` was provoked live.** `mcp_task`, `local_bash`, `local_workflow` and the
  `paused`/`failed`/`killed` transitions are covered by schema-derived fixtures and the stub CLI,
  not by an observed run. The `resource_links` field has never been seen populated here.
- The `task_progress` `usage` object is required by the CLI schema, so it is a value, not a
  pointer. If a future CLI makes it optional, an absent `usage` decodes to the zero struct rather
  than reporting itself absent.
