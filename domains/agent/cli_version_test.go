package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
	sdkerrors "github.com/taumatix/claude-agent-sdk-go/domains/errors"
)

func oldCLI(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a /bin/sh script as the CLI")
	}
	t.Setenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK", "")
	path := filepath.Join(t.TempDir(), "claude")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\ncase \"$1\" in -v) echo '1.0.5 (Claude Code)'; exit 0;; esac\nread -r _\n"), 0o755))
	return path
}

func TestClientConnect_RefusesACLIBelowTheFloorWhenAsked(t *testing.T) {
	opts := agent.DefaultOptions()
	opts.CLIPath = oldCLI(t)
	opts.RequireMinimumCLIVersion = true

	err := agent.NewClient(opts).Connect(context.Background())

	var verr *sdkerrors.CLIVersionError
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, "1.0.5", verr.Found)
}

func TestQuery_RefusesACLIBelowTheFloorWhenAsked(t *testing.T) {
	opts := agent.DefaultOptions()
	opts.CLIPath = oldCLI(t)
	opts.RequireMinimumCLIVersion = true

	var got error
	for _, err := range agent.Query(context.Background(), "hi", opts) {
		got = err
		break
	}

	var verr *sdkerrors.CLIVersionError
	require.ErrorAs(t, got, &verr)
}
