package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

// settingsSeenByCLI starts the stub CLI through the real subprocess transport and returns the
// --settings value it was launched with ("" when the flag is absent).
func settingsSeenByCLI(t *testing.T, opts agent.Options) string {
	t.Helper()
	argsFile := filepath.Join(t.TempDir(), "argv")
	opts.CLIPath = fakeCLIPath
	opts.Env = map[string]string{"FAKECLI_ARGS_FILE": argsFile}
	queryFakeCLIWith(t, opts, "hi")
	raw, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	argv := strings.Split(string(raw), "\n")
	i := slices.Index(argv, "--settings")
	if i < 0 {
		return ""
	}
	return argv[i+1]
}

func ptr[T any](v T) *T { return &v }

func TestE2E_SandboxReachesTheCLIInTheSettingsValue(t *testing.T) {
	got := settingsSeenByCLI(t, agent.Options{Sandbox: &agent.SandboxSettings{
		Enabled:          ptr(true),
		ExcludedCommands: []string{"docker"},
		Network:          &agent.SandboxNetworkConfig{AllowLocalBinding: ptr(false), HTTPProxyPort: ptr(8080)},
	}})
	assert.JSONEq(t, `{"sandbox":{"enabled":true,"excludedCommands":["docker"],`+
		`"network":{"allowLocalBinding":false,"httpProxyPort":8080}}}`, got,
		"an explicit false is sent, an unset field is left out")
}

func TestE2E_SandboxIsMergedIntoSettingsGivenAsJSONOrAsAFile(t *testing.T) {
	sb := &agent.SandboxSettings{Enabled: ptr(true)}

	inline := settingsSeenByCLI(t, agent.Options{Settings: `{"model":"x","sandbox":{"enabled":false}}`, Sandbox: sb})
	assert.JSONEq(t, `{"model":"x","sandbox":{"enabled":true}}`, inline, "the Sandbox option wins over a sandbox key in Settings")

	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"permissions":{"allow":["Read"]}}`), 0o600))
	fromFile := settingsSeenByCLI(t, agent.Options{Settings: path, Sandbox: sb})
	assert.JSONEq(t, `{"permissions":{"allow":["Read"]},"sandbox":{"enabled":true}}`, fromFile)
}

func TestE2E_SettingsAreUntouchedWithoutASandbox(t *testing.T) {
	assert.Equal(t, `{ "model" : "x" }`, settingsSeenByCLI(t, agent.Options{Settings: `{ "model" : "x" }`}))
	assert.Equal(t, "/some/path.json", settingsSeenByCLI(t, agent.Options{Settings: "/some/path.json"}))
	assert.Empty(t, settingsSeenByCLI(t, agent.Options{}))
}

// A Settings value that cannot be merged is an error before any process starts, not settings
// silently dropped.
func TestE2E_UnmergeableSettingsAreRefusedBeforeTheCLIStarts(t *testing.T) {
	sb := &agent.SandboxSettings{Enabled: ptr(true)}
	for name, settings := range map[string]string{
		"missing file":  filepath.Join(t.TempDir(), "nope.json"),
		"not an object": `{"model":`,
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			opts := agent.Options{CLIPath: filepath.Join(t.TempDir(), "never-run"), Settings: settings, Sandbox: sb}

			var gotErr error
			for _, err := range agent.Query(ctx, "hi", opts) {
				gotErr = err
				break
			}
			require.Error(t, gotErr)
			assert.Contains(t, gotErr.Error(), "Options.Settings")

			require.Error(t, agent.NewClient(opts).Connect(ctx))
		})
	}
}
