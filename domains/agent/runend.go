package agent

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
)

// When a Query is over.
//
// A result ends a turn, not necessarily the run: a background agent that
// finishes after it wakes the session for a follow-up turn, whose hooks and
// permission requests still need stdin. The CLI says when no further turn is
// owed by reporting session state "idle". This ports the part of upstream's
// Query._read_messages that waits for it (claude-agent-sdk-python #1088, #1190);
// tracking in-flight agent tasks, for CLIs that report "idle" on every turn or
// not at all, is ROADMAP entry 0b.

const (
	// sdkReadsSessionStateEnv asks the CLI for session_state_changed frames
	// marked sdk_host_only, which the SDK reads and keeps from the caller.
	// claude 2.1.288 honours it; 2.1.283 ignores it and sends nothing.
	sdkReadsSessionStateEnv = "CLAUDE_CODE_SDK_READS_SESSION_STATE"

	// runEndCeilingEnv is the CLI's own bound on waiting for background work
	// once stdin is closed; the SDK bounds its wait for "idle" by the same value,
	// as upstream does.
	runEndCeilingEnv     = "CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS"
	defaultRunEndCeiling = 600 * time.Second
	maxRunEndCeilingMS   = 1<<31 - 1
)

// subprocessEnv is opts.Env with the session-state request added, unless the
// caller chose a value for it themselves.
func subprocessEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env)+1)
	for k, v := range env {
		out[k] = v
	}
	if _, set := out[sdkReadsSessionStateEnv]; !set {
		out[sdkReadsSessionStateEnv] = "1"
	}
	return out
}

// runEndCeiling reads the ceiling as the CLI will see it: from opts.Env if set
// there, otherwise from this process's environment.
func runEndCeiling(env map[string]string) time.Duration {
	raw, ok := env[runEndCeilingEnv]
	if !ok {
		raw, ok = os.LookupEnv(runEndCeilingEnv)
	}
	if !ok {
		return defaultRunEndCeiling
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms < 0 {
		return defaultRunEndCeiling
	}
	if ms > maxRunEndCeilingMS {
		ms = maxRunEndCeilingMS
	}
	return time.Duration(ms) * time.Millisecond
}

// sessionState is the CLI's latest reported state, as far as the messages a
// Query has consumed go. Reported is false until the first
// session_state_changed frame: a CLI that never sends one leaves the result as
// the only end-of-run signal there is.
//
// It is updated by run as it consumes messages, never by the read loop. The read
// loop runs ahead of the consumer, and a state taken from it could end a Query
// with frames before the "idle" still queued: a follow-up result lost, or the
// caller's own "idle" left at the head of the next Query.
type sessionState struct {
	mu       sync.Mutex
	reported bool
	state    messages.SessionState
}

func newSessionState() *sessionState { return &sessionState{} }

func (s *sessionState) set(st messages.SessionState) {
	s.mu.Lock()
	s.reported, s.state = true, st
	s.mu.Unlock()
}

func (s *sessionState) get() (st messages.SessionState, reported bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.reported
}

// runOver reports whether a result just received ends the run.
func (s *sessionState) runOver() bool {
	st, reported := s.get()
	return !reported || st == messages.SessionStateIdle
}

// hostOnly reports whether a session_state_changed frame was sent only because
// the SDK asked for it. The caller did not opt in to those.
func hostOnly(msg *messages.Message) bool {
	if msg.SessionStateChanged == nil || msg.System == nil {
		return false
	}
	var p struct {
		SDKHostOnly bool `json:"sdk_host_only"`
	}
	return json.Unmarshal(msg.System.Raw, &p) == nil && p.SDKHostOnly
}

// run delivers one Query's messages to yield and returns when the run is over:
// at a result once the CLI has said "idle", or has never reported state at all;
// at "idle" after a result; or when the ceiling passes with neither.
func (sm *sessionManager) run(ctx context.Context, yield func(messages.Message, error) bool) {
	var (
		resultSeen bool
		timer      *time.Timer
		ceiling    <-chan time.Time
	)
	stopCeiling := func() {
		if timer != nil {
			timer.Stop()
			timer, ceiling = nil, nil
		}
	}
	defer stopCeiling()

	for {
		select {
		case msg, ok := <-sm.Messages():
			if !ok {
				// msgCh closed — drain errCh for any terminal error.
				select {
				case err := <-sm.Errors():
					if err != nil {
						yield(messages.Message{}, err)
					}
				default:
				}
				return
			}
			if s := msg.SessionStateChanged; s != nil {
				sm.state.set(s.State)
				// Frames the CLI sent because the SDK asked are consumed
				// here, in order, and kept from the caller.
				if !hostOnly(&msg) && !yield(msg, nil) {
					return
				}
				if !resultSeen {
					// "idle" before the result is settled when the result
					// arrives, by runOver.
					continue
				}
				if s.State == messages.SessionStateIdle {
					return
				}
				// A follow-up turn started, or the CLI is waiting on the host:
				// the ceiling only counts the wait between turns, and the next
				// result re-arms it.
				stopCeiling()
				continue
			}
			if !yield(msg, nil) {
				return
			}
			if msg.Result == nil {
				continue
			}
			resultSeen = true
			if sm.state.runOver() {
				return
			}
			stopCeiling()
			timer = time.NewTimer(runEndCeiling(sm.opts.Env))
			ceiling = timer.C

		case <-ceiling:
			return

		case err := <-sm.Errors():
			if err != nil {
				yield(messages.Message{}, err)
			}
			return

		case <-ctx.Done():
			yield(messages.Message{}, ctx.Err())
			return
		}
	}
}
