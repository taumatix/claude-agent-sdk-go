# API compatibility contract

From v1.0.0 this module follows Go's compatibility promise: code that compiles against
v1.x compiles against every later v1.y. Every pull request runs `apidiff` against `main`
and fails on an incompatible change. What follows says exactly what that covers, so you
can write code that keeps working.

## What will not change within v1

- Exported identifiers, signatures, struct fields and constant values.
- Behaviour that a doc comment states as a guarantee.

## What can grow within v1 (write code that tolerates it)

- **Structs gain fields.** Use keyed literals (`agent.Options{Model: "x"}`); unkeyed
  literals and comparing structs with `==` may break.
- **Interfaces do not gain methods.** New capabilities arrive as separate optional
  interfaces, discovered with a type assertion (the `SessionLister` pattern).
- **Open string sets.** Values for `Effort`, `Thinking`, rate-limit status, fast-mode
  state, hook `Source`/`Reason` and similar come from the CLI and can grow. The
  exported constants are the values known today; handle an unknown value.
- **Discriminators.** `messages.Message` and `messages.ContentBlock` are structs keyed by a
  type/subtype string; new kinds can appear in a minor release, so switch with a `default`
  case. `HookInput` is an interface: a new concrete type can appear, and unrecognised hook
  events arrive as `*UnknownHookInput`.
- **Deprecated is not removed.** A deprecated symbol keeps working for all of v1.

## What is not covered

- Wire behaviour of the Claude CLI itself. The CLI floor and the upstream surface this
  module was checked against are in [UPSTREAM.md](UPSTREAM.md); claims not verified
  against a live CLI are marked unverified in doc comments.
- Packages under `shared/`, which are support code rather than the public surface.
- Error message text. Match with `errors.Is` / `errors.As`.
