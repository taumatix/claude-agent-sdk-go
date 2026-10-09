package subprocess_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdkerrors "github.com/taumatix/claude-agent-sdk-go/domains/errors"
	"github.com/taumatix/claude-agent-sdk-go/domains/transport/subprocess"
)

// TestFindCLI_NotFound verifies CLINotFoundError when PATH has no claude binary.
func TestFindCLI_NotFound(t *testing.T) {
	// Use a non-existent explicit path
	_, err := subprocess.New(context.Background(), subprocess.Config{
		CLIPath: "/nonexistent/path/to/claude",
	})
	require.Error(t, err)

	var notFound *sdkerrors.CLINotFoundError
	assert.ErrorAs(t, err, &notFound)
}

// TestFindCLI_EmptyPath_NotFound verifies CLINotFoundError when PATH is empty and no fallback exists.
func TestFindCLI_EmptyPath_NotFound(t *testing.T) {
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", "/nonexistent_path_that_surely_does_not_exist")

	_, err := subprocess.New(context.Background(), subprocess.Config{})
	// May or may not find claude in well-known locations, but if not found should be CLINotFoundError
	if err != nil {
		var notFound *sdkerrors.CLINotFoundError
		assert.ErrorAs(t, err, &notFound, "expected CLINotFoundError, got: %v", err)
	}
}

// TestSubprocessTransport_Integration requires a real claude binary.
// Skipped unless RUN_INTEGRATION=1.
func TestSubprocessTransport_Integration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("Set RUN_INTEGRATION=1 to run integration tests")
	}

	ctx := context.Background()
	tr, err := subprocess.New(ctx, subprocess.Config{})
	require.NoError(t, err)
	defer tr.Close()

	// Transport should be usable
	assert.NotNil(t, tr)
}

func scriptTransport(t *testing.T, body string) *subprocess.Transport {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a /bin/sh script as the CLI")
	}
	t.Setenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK", "1")
	path := filepath.Join(t.TempDir(), "claude")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	tr, err := subprocess.New(context.Background(), subprocess.Config{CLIPath: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })
	return tr
}

func TestReceive_ReturnsWhenContextIsCancelledAndTheProcessIsSilent(t *testing.T) {
	tr := scriptTransport(t, "read -r _")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := tr.Receive(ctx)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestReceive_ACancelledReceiveDoesNotLoseTheNextLine(t *testing.T) {
	tr := scriptTransport(t, `sleep 0.4; echo '{"n":1}'; echo '{"n":2}'; read -r _`)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := tr.Receive(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	for _, want := range []string{`{"n":1}`, `{"n":2}`} {
		line, err := tr.Receive(context.Background())
		require.NoError(t, err)
		assert.Equal(t, want, string(line))
	}
}

func TestReceive_EOFIsStickyAfterTheProcessExits(t *testing.T) {
	tr := scriptTransport(t, `echo '{"n":1}'`)

	line, err := tr.Receive(context.Background())
	require.NoError(t, err)
	assert.Equal(t, `{"n":1}`, string(line))
	for i := 0; i < 3; i++ {
		_, err = tr.Receive(context.Background())
		assert.ErrorIs(t, err, io.EOF)
	}
}

func TestReceive_CloseUnblocksAReceiveWithNoDeadline(t *testing.T) {
	tr := scriptTransport(t, "read -r _")

	errCh := make(chan error, 1)
	go func() {
		_, err := tr.Receive(context.Background())
		errCh <- err
	}()
	time.Sleep(100 * time.Millisecond)
	go func() { _ = tr.Close() }()

	select {
	case err := <-errCh:
		assert.Error(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("Receive stayed blocked after Close")
	}
}

func TestSend_ReturnsWhenContextIsCancelledAndTheProcessNeverReadsStdin(t *testing.T) {
	tr := scriptTransport(t, "sleep 30")
	big := []byte(strings.Repeat("x", 4<<20))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := tr.Send(ctx, big)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestSend_AlreadyCancelledContextWritesNothing(t *testing.T) {
	tr := scriptTransport(t, "read -r _")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, tr.Send(ctx, []byte(`{"a":1}`)), context.Canceled)
}

func TestSend_CloseUnblocksAStuckSendWithoutAContextDeadline(t *testing.T) {
	tr := scriptTransport(t, "sleep 30")
	errc := make(chan error, 1)
	go func() { errc <- tr.Send(context.Background(), []byte(strings.Repeat("x", 4<<20))) }()
	time.Sleep(200 * time.Millisecond)

	closed := make(chan struct{})
	go func() { _ = tr.Close(); close(closed) }()

	select {
	case err := <-errc:
		assert.Error(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Send was not released by Close")
	}
	select {
	case <-closed:
	case <-time.After(20 * time.Second):
		t.Fatal("Close did not return")
	}
}

func TestSend_DeliversALineToAProcessThatReads(t *testing.T) {
	tr := scriptTransport(t, `read -r line; echo "got:$line"; read -r _`)
	require.NoError(t, tr.Send(context.Background(), []byte("hello")))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	line, err := tr.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, "got:hello", string(line))
}

func versionStub(t *testing.T, version string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a /bin/sh script as the CLI")
	}
	t.Setenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK", "")
	path := filepath.Join(t.TempDir(), "claude")
	body := "#!/bin/sh\ncase \"$1\" in -v) echo '" + version + "'; exit 0;; esac\nread -r _\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
	return path
}

func TestNew_ACLIBelowTheFloorIsRefusedWhenEnforced(t *testing.T) {
	path := versionStub(t, "1.9.3 (Claude Code)")
	tr, err := subprocess.New(context.Background(), subprocess.Config{CLIPath: path, EnforceMinimumVersion: true})
	if tr != nil {
		_ = tr.Close()
	}
	var verr *sdkerrors.CLIVersionError
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, "1.9.3", verr.Found)
	assert.Equal(t, subprocess.MinimumCLIVersion, verr.Required)
	assert.Equal(t, path, verr.CLIPath)
	assert.Nil(t, tr)
}

func TestNew_ACLIBelowTheFloorStillStartsByDefault(t *testing.T) {
	path := versionStub(t, "1.9.3 (Claude Code)")
	tr, err := subprocess.New(context.Background(), subprocess.Config{CLIPath: path})
	require.NoError(t, err)
	require.NoError(t, tr.Close())
}

func TestNew_ACLIAtOrAboveTheFloorStartsWhenEnforced(t *testing.T) {
	for _, v := range []string{"2.0.0 (Claude Code)", "2.1.283 (Claude Code)", "10.0.0"} {
		path := versionStub(t, v)
		tr, err := subprocess.New(context.Background(), subprocess.Config{CLIPath: path, EnforceMinimumVersion: true})
		require.NoError(t, err, v)
		require.NoError(t, tr.Close())
	}
}

func TestNew_AnUnreadableVersionIsNotRefusedWhenEnforced(t *testing.T) {
	path := versionStub(t, "not a version")
	tr, err := subprocess.New(context.Background(), subprocess.Config{CLIPath: path, EnforceMinimumVersion: true})
	require.NoError(t, err)
	require.NoError(t, tr.Close())
}

func TestNew_TheSkipVariableDisablesEnforcement(t *testing.T) {
	path := versionStub(t, "1.0.0")
	t.Setenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK", "1")
	tr, err := subprocess.New(context.Background(), subprocess.Config{CLIPath: path, EnforceMinimumVersion: true})
	require.NoError(t, err)
	require.NoError(t, tr.Close())
}

func receiveUntil(t *testing.T, tr *subprocess.Transport, stop string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var got []string
	for {
		line, err := tr.Receive(ctx)
		require.NoError(t, err)
		got = append(got, string(line))
		if string(line) == stop {
			return got
		}
	}
}

func TestSend_ACancelledSendThatNeverStartedWritingLeavesTheTransportUsable(t *testing.T) {
	tr := scriptTransport(t, `sleep 1; while IFS= read -r l; do echo "len:${#l}"; done`)

	first := make(chan error, 1)
	go func() { first <- tr.Send(context.Background(), []byte(strings.Repeat("a", 200000))) }()
	time.Sleep(200 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, tr.Send(ctx, []byte("bbbbb")), context.DeadlineExceeded)

	require.NoError(t, <-first)
	require.NoError(t, tr.Send(context.Background(), []byte("ccc")))
	got := receiveUntil(t, tr, "len:3")
	assert.Equal(t, []string{"len:200000", "len:3"}, got, "the abandoned line must never reach the process")
}

func TestSend_ACancelledSendWhoseWriteLandedNothingLeavesTheTransportUsable(t *testing.T) {
	tr := scriptTransport(t, `sleep 1; while IFS= read -r l; do echo "$l"; done`)
	pad := strings.Repeat("p", 250) // under PIPE_BUF everywhere, so a write is whole or nothing

	var sent []string
	var timedOut string
	for i := 0; i < 100000; i++ {
		line := fmt.Sprintf("%06d%s", i, pad)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		err := tr.Send(ctx, []byte(line))
		cancel()
		if err != nil {
			require.ErrorIs(t, err, context.DeadlineExceeded)
			timedOut = line[:6]
			break
		}
		sent = append(sent, line[:6])
	}
	require.NotEmpty(t, timedOut, "the pipe never filled")

	require.NoError(t, tr.Send(context.Background(), []byte("marker")), "a send that landed nothing must not poison the stream")
	got := receiveUntil(t, tr, "marker")
	var ids []string
	for _, l := range got[:len(got)-1] {
		ids = append(ids, l[:6])
	}
	assert.Equal(t, sent, ids, "every acknowledged line arrives once, and the cancelled one does not")
	assert.NotContains(t, ids, timedOut)
}
