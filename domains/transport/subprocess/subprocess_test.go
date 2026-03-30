package subprocess_test

import (
	"context"
	"os"
	"testing"

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
