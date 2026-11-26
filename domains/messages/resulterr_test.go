package messages_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
)

func TestResultErr_NilForSuccessAndNilReceiver(t *testing.T) {
	assert.NoError(t, (&messages.ResultMessage{}).Err())
	var r *messages.ResultMessage
	assert.NoError(t, r.Err())
}

func TestResultErr_CarriesTheResult(t *testing.T) {
	status := 529
	r := &messages.ResultMessage{IsError: true, Subtype: "success", APIErrorStatus: &status, Result: "overloaded"}
	err := r.Err()
	var rerr *messages.ResultError
	require.True(t, errors.As(err, &rerr))
	assert.Same(t, r, rerr.Result)
	assert.Equal(t, `claude result error (subtype "success", API status 529): overloaded`, err.Error())
}

func TestResultErr_PrefersTheErrorsList(t *testing.T) {
	r := &messages.ResultMessage{IsError: true, Subtype: "error_max_turns", Errors: []string{"too many turns"}, Result: "x"}
	assert.Equal(t, `claude result error (subtype "error_max_turns"): too many turns`, r.Err().Error())
}
