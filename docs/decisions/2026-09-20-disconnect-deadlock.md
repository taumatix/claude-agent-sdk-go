# Disconnect blocked until the caller's context expired

## Goal

Make `Client.Disconnect()` return when the session is shut down, rather than
when the caller's context happens to expire.

## What we saw

The end-to-end test added alongside it took exactly 30.0s per case — the
context deadline, to three decimal places. Timing the calls separately against
the stub CLI, with a 120s context:

    Connect took 59.135583ms
    Query took 301.042µs
    Disconnect took 1m59.9453555s

Not slowness. `Disconnect` was waiting out the whole context, whatever it was
set to. On a `context.Background()` it never returns.

## Root cause

`sessionManager.Close()` was:

    sm.cancel()
    sm.wg.Wait()
    return sm.transport.Close()

`readLoop` is parked in `transport.Receive` → `jsonlines.Reader.ReadLine()`, a
blocking pipe read that ignores the context. `sm.cancel()` does not interrupt
it. The CLI holds its stdout open for as long as its stdin is open, and stdin
is closed by `transport.Close()` — which runs *after* `wg.Wait()`. So each side
waits for the other. It only broke when `exec.CommandContext` killed the child
on context expiry, which is what made the duration equal the timeout.

This is not an artefact of the stub. The real `claude` in stream-json input mode
also stays alive waiting for more input after emitting a result, so it holds the
same deadlock.

## Approach chosen

Close the transport before waiting, keeping the cancel first:

    sm.cancel()
    err := sm.transport.Close()
    sm.wg.Wait()
    return err

Closing stdin ends the CLI, its stdout closes, the read returns, `readLoop`
exits. Cancelling first is what keeps the resulting read error quiet —
`readLoop` reports a cancelled context as a clean shutdown rather than as a
`ProcessError`, so the reordering does not start emitting spurious errors.

## Alternatives considered

- **Make `Receive` context-aware**, so `cancel()` unblocks the read. Rejected
  for now: it means a reader goroutine per transport and a channel handoff,
  changing the `transport.Transport` contract for every implementation. It
  would also leave `Close` closing stdin after the wait, which is the wrong
  order regardless. Worth doing on its own terms — it is on the roadmap.
- **Time-bound the wait** (`wg.Wait()` with a timeout, then proceed). Rejected:
  it hides the deadlock behind a shorter delay instead of removing it, and
  leaks the read goroutine.

## Libraries added or used

None.

## Trade-offs and risks

- `cmd.Wait()` inside `Transport.Close()` now runs while `readLoop` may still be
  reading stdout, and `Wait` closes that pipe. The read can therefore return
  `os.ErrClosed` rather than `io.EOF`. Harmless here because the context is
  already cancelled and `readLoop` treats that as a clean end — but it is the
  reason the cancel must stay first. The suite passes under `-race`.
- The end-to-end suite went from 91s to 2.3s, which is the same finding stated
  as a number.
