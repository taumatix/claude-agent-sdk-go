package messages

import "fmt"

// ResultError is a failed ResultMessage as an error, for callers who prefer to
// return it. This SDK never yields it on its own: a result with IsError set is
// delivered as a ResultMessage and the stream ends cleanly, and that is a v1
// guarantee. Obtain one with ResultMessage.Err.
type ResultError struct {
	// Result is the failed result, whole.
	Result *ResultMessage
}

func (e *ResultError) Error() string {
	r := e.Result
	msg := fmt.Sprintf("claude result error (subtype %q", r.Subtype)
	if r.APIErrorStatus != nil {
		msg += fmt.Sprintf(", API status %d", *r.APIErrorStatus)
	}
	msg += ")"
	switch {
	case len(r.Errors) > 0:
		msg += ": " + r.Errors[0]
	case r.Result != "":
		msg += ": " + r.Result
	}
	return msg
}

// Err returns a *ResultError when IsError is set, nil otherwise. Use it with
// errors.As to recover the result.
func (r *ResultMessage) Err() error {
	if r == nil || !r.IsError {
		return nil
	}
	return &ResultError{Result: r}
}
