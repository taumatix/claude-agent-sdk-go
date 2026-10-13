package messages

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOriginKeepsEveryFieldTheCLISends(t *testing.T) {
	o, err := ParseOrigin(json.RawMessage(`{"kind":"peer","from":"unix:/s","name":"Ana","fromSession":"s-1",` +
		`"senderTaskId":"tk-9","body":"hi","verifiedPeerPid":4242}`))
	require.NoError(t, err)
	require.NotNil(t, o)
	assert.Equal(t, OriginPeer, o.Kind)
	assert.Equal(t, "unix:/s", o.From)
	assert.Equal(t, "Ana", o.Name)
	assert.Equal(t, "s-1", o.FromSession)
	assert.Equal(t, "tk-9", o.SenderTaskID)
	assert.Equal(t, "hi", o.Body)
	require.NotNil(t, o.VerifiedPeerPID)
	assert.Equal(t, 4242, *o.VerifiedPeerPID)
	assert.False(t, o.IsHuman())
}

func TestParseOriginSubkindServerAndUnknownKinds(t *testing.T) {
	o, err := ParseOrigin(json.RawMessage(`{"kind":"task-notification","subkind":"scheduled-trigger"}`))
	require.NoError(t, err)
	assert.Equal(t, SubkindScheduledTrigger, o.Subkind)

	o, err = ParseOrigin(json.RawMessage(`{"kind":"channel","server":"slack"}`))
	require.NoError(t, err)
	assert.Equal(t, "slack", o.Server)

	o, err = ParseOrigin(json.RawMessage(`{"kind":"from-the-future"}`))
	require.NoError(t, err)
	assert.Equal(t, OriginKind("from-the-future"), o.Kind, "a kind this SDK has not heard of is kept")
	assert.False(t, o.IsHuman(), "and is not human")
}

func TestAnUnattributedMessageHasNoOriginAndIsNotAnError(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, {}, json.RawMessage("null")} {
		o, err := ParseOrigin(raw)
		require.NoError(t, err)
		assert.Nil(t, o)
		assert.False(t, o.IsHuman())
	}
	_, err := ParseOrigin(json.RawMessage(`[1]`))
	assert.Error(t, err)
}

func TestParsedOriginOnUserAndResultMessages(t *testing.T) {
	raw := json.RawMessage(`{"kind":"human"}`)
	uo, err := UserMessage{Origin: raw}.ParsedOrigin()
	require.NoError(t, err)
	assert.True(t, uo.IsHuman())
	ro, err := ResultMessage{Origin: raw}.ParsedOrigin()
	require.NoError(t, err)
	assert.True(t, ro.IsHuman())
}
