# Model-internal content blocks arrive whole, not as phantom empty text

## Goal

Stop the SDK turning every content block it does not model — including every
server-side tool result the CLI actually emits — into an empty `TextBlock`.

## What we believed, and what we found

The SDK matched `server_tool_result` as the wire type for a server-side tool
result. Nothing emits that name. Checked against the vendor's own artefact: the
content-block switch inside the `claude` binary at `2.1.220` enumerates

    server_tool_use, web_search_tool_result, web_fetch_tool_result,
    advisor_tool_result, code_execution_tool_result,
    bash_code_execution_tool_result, text_editor_code_execution_tool_result,
    tool_search_tool_result, mcp_tool_use, mcp_tool_result, container_upload,
    compaction

`advisor_tool_result` appears 24 times in that binary; `server_tool_result`
appears once, inside an English sentence in a prompt. The Python SDK hit the
same bug and fixed it in its #836 — its parser matches `advisor_tool_result`.

So `case protocol.ContentTypeServerToolResult` never fired, and every such
block fell to the `default` branch, which returned
`ContentBlock{Text: &TextBlock{Text: ""}}`. Two separate harms: the payload was
destroyed, and the caller was handed a text block the model never wrote. An
assistant turn whose only content was a web search result arrived looking like
the model had answered with nothing.

The existing test passed throughout, because it encoded `server_tool_result` on
both sides — the round-trip proved self-consistency, not correctness.

## Approach chosen

1. Name the seven real result types as constants, with `IsServerToolResult` as
   the predicate. The set is closed and small, so a map plus a predicate beats
   an exported slice callers could mutate.
2. Add `ServerToolResultBlock.Type`, carrying the wire type. Without it a caller
   holding the block cannot tell which server tool ran, and `Content`'s schema
   differs per tool — so the block would be undecodable even once delivered.
3. Add `ContentBlock.Unknown` (`UnknownBlock{Type, Raw}`) for anything else.
   `Raw` is the block verbatim, so a caller can decode a block type added after
   this release without waiting for an SDK version.
4. Keep `ContentTypeServerToolResult` as a deprecated constant that still
   decodes, so code and fixtures built on it compile and behave.

## Alternatives considered

- **Map only `advisor_tool_result`, as the Python SDK does.** Rejected: the CLI
  emits six other result types the Python SDK still drops. Matching upstream's
  coverage would mean knowingly shipping the same gap.
- **Keep returning an empty `TextBlock` for unknown blocks, to avoid a
  behaviour change.** Rejected: that behaviour is the bug. A caller cannot
  distinguish it from real empty text, so nothing can depend on it usefully.
- **Infer `IsError` from a `content.type` ending in `_tool_result_error`.**
  Rejected: the vendor documents no such rule; it happens to hold for
  `advisor_tool_result_error` today. Documented on the field instead.

## Libraries added or used

None. `encoding/json`'s `RawMessage` already carries the deferred payloads.

## Trade-offs and risks

- **Behaviour change, not an API break.** The public API is additive (two new
  fields, seven new constants, one new function; nothing removed or retyped),
  so `apidiff` stays quiet. But a caller that today reads an empty `TextBlock`
  for an unmodelled block will get `Unknown` instead. Called out in the
  CHANGELOG under Fixed.
- **The list of server tools will grow.** A new one is not a silent loss any
  more — it arrives as `Unknown` with its payload — but it will not be typed as
  a server tool result until the constant is added.
- **No live-CLI coverage of these blocks.** A server-side tool call cannot be
  provoked on demand from a real `claude`, and CI has no credentials. The
  end-to-end test drives a real subprocess over real pipes with a stub
  counterparty; the block names in that stub come from the binary, not from
  recall.
