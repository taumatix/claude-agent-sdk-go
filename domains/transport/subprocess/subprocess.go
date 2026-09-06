// Package subprocess implements a Transport that communicates with the Claude Code CLI
// via stdin/stdout using newline-delimited JSON.
package subprocess

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	sdkerrors "github.com/taumatix/claude-agent-sdk-go/domains/errors"
	"github.com/taumatix/claude-agent-sdk-go/shared/jsonlines"
)

const (
	// MinimumCLIVersion is the minimum supported Claude Code CLI version.
	MinimumCLIVersion = "2.0.0"

	shutdownGracePeriod = 5 * time.Second
	versionCheckTimeout = 2 * time.Second

	sdkVersion = "0.1.0"
)

// Config holds the configuration for creating a subprocess Transport.
type Config struct {
	// CLIPath is the path to the claude binary. Empty means auto-detect.
	CLIPath string

	// Args are the CLI flag arguments (NOT including the binary path itself).
	Args []string

	// Env entries are merged over the inherited environment. CLAUDECODE is always stripped.
	Env map[string]string

	// WorkingDirectory sets the working directory for the subprocess.
	WorkingDirectory string
}

// Transport implements transport.Transport by spawning the claude CLI as a subprocess
// and communicating over its stdin/stdout.
type Transport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *jsonlines.Reader
	writer *jsonlines.Writer
	mu     sync.Mutex
	once   sync.Once
}

// New creates a new subprocess Transport. It locates the CLI binary, optionally checks
// its version, builds the command, and starts the process.
func New(ctx context.Context, cfg Config) (*Transport, error) {
	cliPath, err := findCLI(cfg.CLIPath)
	if err != nil {
		return nil, err
	}

	if os.Getenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK") == "" {
		checkVersion(cliPath)
	}

	// Build subprocess environment: inherit minus CLAUDECODE, plus SDK markers, plus cfg.Env.
	env := buildEnv(cfg.Env)

	cmd := exec.CommandContext(ctx, cliPath, cfg.Args...)
	cmd.Env = env
	if cfg.WorkingDirectory != "" {
		cmd.Dir = cfg.WorkingDirectory
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, &sdkerrors.CLIConnectionError{Msg: "failed to create stdin pipe: " + err.Error()}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, &sdkerrors.CLIConnectionError{Msg: "failed to create stdout pipe: " + err.Error()}
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, &sdkerrors.CLIConnectionError{Msg: "failed to start claude: " + err.Error()}
	}

	t := &Transport{
		cmd:    cmd,
		stdin:  stdin,
		reader: jsonlines.NewReader(stdout),
		writer: jsonlines.NewWriter(stdin),
	}
	return t, nil
}

// Send writes data to the subprocess stdin as a JSON line.
func (t *Transport) Send(ctx context.Context, data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.writer.WriteLine(data)
}

// Receive returns the next line from the subprocess stdout.
// Returns io.EOF when the stream ends.
func (t *Transport) Receive(ctx context.Context) ([]byte, error) {
	return t.reader.ReadLine()
}

// Close shuts down the subprocess gracefully: close stdin → wait 5s → SIGTERM → wait 5s → SIGKILL.
func (t *Transport) Close() error {
	var closeErr error
	t.once.Do(func() {
		// Close stdin to signal EOF to the subprocess
		t.mu.Lock()
		_ = t.stdin.Close()
		t.mu.Unlock()

		// Wait for graceful exit
		done := make(chan error, 1)
		go func() {
			done <- t.cmd.Wait()
		}()

		select {
		case <-done:
			return
		case <-time.After(shutdownGracePeriod):
		}

		// Graceful period expired — send SIGTERM
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Signal(os.Interrupt)
		}

		select {
		case <-done:
			return
		case <-time.After(shutdownGracePeriod):
		}

		// Still running — SIGKILL
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		<-done
	})
	return closeErr
}

// findCLI locates the claude binary, checking in order:
// explicit path → PATH lookup → well-known locations.
func findCLI(cliPath string) (string, error) {
	if cliPath != "" {
		if _, err := os.Stat(cliPath); err == nil {
			return cliPath, nil
		}
		return "", &sdkerrors.CLINotFoundError{
			Msg:     "Claude Code not found at specified path",
			CLIPath: cliPath,
		}
	}

	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".npm-global", "bin", "claude"),
		"/usr/local/bin/claude",
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, "node_modules", ".bin", "claude"),
		filepath.Join(home, ".claude", "local", "claude"),
	}
	if runtime.GOOS == "windows" {
		for i, c := range candidates {
			candidates[i] = c + ".exe"
		}
	}

	for _, p := range candidates {
		info, err := os.Stat(p)
		if err == nil && !info.IsDir() {
			return p, nil
		}
	}

	return "", &sdkerrors.CLINotFoundError{
		Msg: "Claude Code not found. Install with:\n" +
			"  npm install -g @anthropic-ai/claude-code\n" +
			"\nIf already installed locally, try:\n" +
			"  export PATH=\"$HOME/node_modules/.bin:$PATH\"\n" +
			"\nOr provide the path via Options.CLIPath.",
	}
}

// checkVersion runs claude -v and logs a warning if below MinimumCLIVersion. Non-fatal.
func checkVersion(cliPath string) {
	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, cliPath, "-v").Output()
	if err != nil {
		return
	}

	re := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	m := re.FindStringSubmatch(string(out))
	if m == nil {
		return
	}

	var major, minor, patch int
	fmt.Sscanf(m[1], "%d", &major)
	fmt.Sscanf(m[2], "%d", &minor)
	fmt.Sscanf(m[3], "%d", &patch)

	const minMajor, minMinor, minPatch = 2, 0, 0
	if major < minMajor || (major == minMajor && minor < minMinor) || (major == minMajor && minor == minMinor && patch < minPatch) {
		log.Printf("WARNING: Claude Code version %d.%d.%d is below minimum required %s. Some features may not work correctly.",
			major, minor, patch, MinimumCLIVersion)
	}
}

// buildEnv constructs the subprocess environment.
func buildEnv(extra map[string]string) []string {
	inherited := os.Environ()
	filtered := make([]string, 0, len(inherited))
	for _, e := range inherited {
		// Strip CLAUDECODE so SDK-spawned subprocesses don't think they're
		// running inside a Claude Code parent.
		if len(e) >= 10 && e[:10] == "CLAUDECODE" && (len(e) == 10 || e[10] == '=') {
			continue
		}
		filtered = append(filtered, e)
	}

	// Build override map to deduplicate
	overrides := map[string]string{
		"CLAUDE_CODE_ENTRYPOINT":   "sdk-go",
		"CLAUDE_AGENT_SDK_VERSION": sdkVersion,
	}
	for k, v := range extra {
		overrides[k] = v
	}

	// Apply overrides: remove existing keys that will be overridden
	result := make([]string, 0, len(filtered)+len(overrides))
	for _, e := range filtered {
		skip := false
		for k := range overrides {
			prefix := k + "="
			if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
				skip = true
				break
			}
		}
		if !skip {
			result = append(result, e)
		}
	}
	for k, v := range overrides {
		result = append(result, k+"="+v)
	}
	return result
}
