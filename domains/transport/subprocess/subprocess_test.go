package subprocess_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	tr := scriptTransport(t, "exec cat >/dev/null")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := tr.Receive(ctx)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestReceive_ACancelledReceiveDoesNotLoseTheNextLine(t *testing.T) {
	tr := scriptTransport(t, `sleep 0.4; echo '{"n":1}'; echo '{"n":2}'; exec cat >/dev/null`)

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
	tr := scriptTransport(t, "exec cat >/dev/null")

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
