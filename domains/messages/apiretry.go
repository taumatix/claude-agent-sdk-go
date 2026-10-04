package messages

import (
	"encoding/json"
	"time"

	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

// APIRetryMessage reports that an API request failed with a retryable error
// and the CLI will try again after RetryDelay. A run that keeps producing
// these is stalled on the API (overloaded, rate-limited, or unreachable),
// which is otherwise invisible until the run fails or finishes late.
type APIRetryMessage struct {
	// Attempt counts from 1; MaxRetries is the cap the CLI is working to.
	Attempt    int
	MaxRetries int
	RetryDelay time.Duration
	// ErrorStatus is the HTTP status, or 0 when the CLI reports none: a
	// timeout, a refused connection, or a response it could not read as an
	// API error.
	ErrorStatus int
	// Error is the CLI's short kind for the failure, such as "overloaded" or
	// "unknown".
	Error string
	// NoResponse is set when the API sent no response headers within the
	// first-byte window. For this cause MaxRetries is its own cap, normally
	// one retry, rather than the session's.
	NoResponse *APIRetryNoResponse
	UUID       string
	SessionID  string
}

// APIRetryNoResponse is the first-byte timeout behind an APIRetryMessage.
type APIRetryNoResponse struct {
	// Waited is how long the failed attempt waited for response headers.
	Waited time.Duration
	// RetryWait is how long the retry will wait for them.
	RetryWait time.Duration
}

func apiRetryFromWire(p protocol.APIRetryPayload) *APIRetryMessage {
	m := &APIRetryMessage{
		Attempt:    p.Attempt,
		MaxRetries: p.MaxRetries,
		RetryDelay: time.Duration(p.RetryDelayMS) * time.Millisecond,
		UUID:       p.UUID,
		SessionID:  p.SessionID,
	}
	if p.ErrorStatus != nil {
		m.ErrorStatus = *p.ErrorStatus
	}
	var kind string
	if json.Unmarshal(p.Error, &kind) == nil {
		m.Error = kind
	} else if len(p.Error) > 0 && string(p.Error) != "null" {
		m.Error = string(p.Error)
	}
	if nr := p.NoResponse; nr != nil {
		m.NoResponse = &APIRetryNoResponse{
			Waited:    time.Duration(nr.WaitedMS) * time.Millisecond,
			RetryWait: time.Duration(nr.RetryWaitMS) * time.Millisecond,
		}
	}
	return m
}
