package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
	"github.com/taumatix/claude-agent-sdk-go/domains/sessions"
)

// The stub CLI sends transcript_mirror frames on the real pipes, as the CLI does
// under --session-mirror, and the store sits behind the real Options. Not
// covered: a live CLI (the frame shape is from upstream's source, not captured).

type recordingStore struct {
	*sessions.MemorySessionStore
	mu     sync.Mutex
	calls  []sessions.SessionKey
	failN  int // fail this many calls, then succeed; -1 fails for ever
	failed int
}

func (r *recordingStore) Append(ctx context.Context, key sessions.SessionKey, entries []sessions.SessionStoreEntry) error {
	r.mu.Lock()
	r.calls = append(r.calls, key)
	fail := r.failN < 0 || r.failed < r.failN
	if fail {
		r.failed++
	}
	r.mu.Unlock()
	if fail {
		return errors.New("store unavailable")
	}
	return r.MemorySessionStore.Append(ctx, key, entries)
}

func mirrorOpts(t *testing.T, scenario string, store sessions.SessionStore) (agent.Options, string) {
	t.Helper()
	argsFile := filepath.Join(t.TempDir(), "args")
	opts := fakeOpts(scenario, map[string]string{
		"CLAUDE_CONFIG_DIR": t.TempDir(),
		"FAKECLI_ARGS_FILE": argsFile,
	})
	opts.SessionStore = store
	return opts, argsFile
}

func runMirror(t *testing.T, opts agent.Options) []messages.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return collect(t, agent.Query(ctx, "hi", opts))
}

func uuids(entries []sessions.SessionStoreEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e["uuid"].(string))
	}
	return out
}

func TestE2E_TranscriptFramesReachTheSessionStore(t *testing.T) {
	store := &recordingStore{MemorySessionStore: sessions.NewMemorySessionStore()}
	opts, argsFile := mirrorOpts(t, "mirror", store)

	got := runMirror(t, opts)

	args, _ := os.ReadFile(argsFile)
	assert.Contains(t, string(args), "--session-mirror")
	assert.Equal(t, []string{"Go 1.26 is out."}, results(got))
	for _, m := range got {
		assert.False(t, m.System != nil && m.System.Subtype == "mirror_error", "no error expected")
	}

	main, err := store.Load(context.Background(), sessions.SessionKey{ProjectKey: "-work-app", SessionID: "sess-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"m1", "m2", "m3"}, uuids(main))
	sub, err := store.Load(context.Background(), sessions.SessionKey{ProjectKey: "-work-app", SessionID: "sess-1", Subpath: "subagents/agent-7"})
	require.NoError(t, err)
	assert.Equal(t, []string{"s1"}, uuids(sub))

	assert.Len(t, store.calls, 2, "batched mode appends once per file, not once per frame")
}

func TestE2E_EagerFlushAppendsEveryFrame(t *testing.T) {
	store := &recordingStore{MemorySessionStore: sessions.NewMemorySessionStore()}
	opts, _ := mirrorOpts(t, "mirror", store)
	opts.SessionStoreFlush = agent.SessionStoreFlushEager

	runMirror(t, opts)

	assert.Len(t, store.calls, 3)
	main, err := store.Load(context.Background(), sessions.SessionKey{ProjectKey: "-work-app", SessionID: "sess-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"m1", "m2", "m3"}, uuids(main))
}

func TestE2E_ATransientStoreFailureIsRetriedAndInvisible(t *testing.T) {
	store := &recordingStore{MemorySessionStore: sessions.NewMemorySessionStore(), failN: 2}
	opts, _ := mirrorOpts(t, "mirror", store)

	got := runMirror(t, opts)

	for _, m := range got {
		assert.False(t, m.System != nil && m.System.Subtype == "mirror_error", "a retry that succeeded must not be reported")
	}
	main, err := store.Load(context.Background(), sessions.SessionKey{ProjectKey: "-work-app", SessionID: "sess-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"m1", "m2", "m3"}, uuids(main))
}

func TestE2E_AStoreThatKeepsFailingIsReportedAndTheSessionGoesOn(t *testing.T) {
	store := &recordingStore{MemorySessionStore: sessions.NewMemorySessionStore(), failN: -1}
	opts, _ := mirrorOpts(t, "mirror", store)

	got := runMirror(t, opts)

	var errs []messages.Message
	resultAt, errAt := -1, -1
	for i, m := range got {
		if m.System != nil && m.System.Subtype == "mirror_error" {
			errs = append(errs, m)
			if errAt < 0 {
				errAt = i
			}
		}
		if m.Result != nil {
			resultAt = i
		}
	}
	require.Len(t, errs, 2, "one report per file whose batch was dropped")
	assert.Less(t, errAt, resultAt, "the report must reach the caller before the result it belongs to")
	assert.Equal(t, []string{"Go 1.26 is out."}, results(got), "the session is unaffected")
	assert.Len(t, store.calls, 6, "three attempts for each of two files")

	var body struct {
		Error string `json:"error"`
		Key   struct {
			ProjectKey string `json:"project_key"`
			SessionID  string `json:"session_id"`
		} `json:"key"`
	}
	require.NoError(t, json.Unmarshal(errs[0].System.Raw, &body))
	assert.Equal(t, "store unavailable", body.Error)
	assert.Equal(t, "-work-app", body.Key.ProjectKey)
	assert.Equal(t, "sess-1", body.Key.SessionID)
}

func TestE2E_AFrameOutsideTheProjectsDirIsReportedNotStored(t *testing.T) {
	store := &recordingStore{MemorySessionStore: sessions.NewMemorySessionStore()}
	opts, _ := mirrorOpts(t, "mirror-outside", store)

	got := runMirror(t, opts)

	var found bool
	for _, m := range got {
		if m.System != nil && m.System.Subtype == "mirror_error" {
			found = true
			assert.Contains(t, string(m.System.Raw), "CLAUDE_CONFIG_DIR")
		}
	}
	assert.True(t, found)
	assert.Empty(t, store.calls)
}

func TestE2E_WithoutAStoreNoMirrorIsRequestedAndFramesAreIgnored(t *testing.T) {
	opts, argsFile := mirrorOpts(t, "mirror", nil)

	got := runMirror(t, opts)

	args, _ := os.ReadFile(argsFile)
	assert.False(t, strings.Contains(string(args), "--session-mirror"))
	assert.Equal(t, []string{"Go 1.26 is out."}, results(got))
}
