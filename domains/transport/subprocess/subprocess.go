// Package subprocess implements a Transport that communicates with the Claude Code CLI
// via stdin/stdout using newline-delimited JSON.
package subprocess

import (
	"context"
	"errors"
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

	sdkVersion = "0.3.0"
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

	// EnforceMinimumVersion makes a CLI older than MinimumCLIVersion an error
	// (*errors.CLIVersionError) instead of a logged warning. A CLI whose version cannot be
	// read is let through either way. CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK still disables the check.
	EnforceMinimumVersion bool
}

// Transport implements transport.Transport by spawning the claude CLI as a subprocess
// and communicating over its stdin/stdout.
type Transport struct {
	cmd    *exec.Cmd
	stdin  *os.File
	reader *jsonlines.Reader
	mu     sync.Mutex
	once   sync.Once

	stdinOnce sync.Once

	lines   chan []byte
	readErr error
	done    chan struct{}
}

// New creates a new subprocess Transport. It locates the CLI binary, optionally checks
// its version, builds the command, and starts the process.
func New(ctx context.Context, cfg Config) (*Transport, error) {
	cliPath, err := findCLI(cfg.CLIPath)
	if err != nil {
		return nil, err
	}

	if os.Getenv("CLAUDE_AGENT_SDK_SKIP_VERSION_CHECK") == "" {
		if err := checkVersion(cliPath, cfg.EnforceMinimumVersion); err != nil {
			return nil, err
		}
	}

	// Build subprocess environment: inherit minus CLAUDECODE, plus SDK markers, plus cfg.Env.
	env := buildEnv(cfg.Env)

	cmd := exec.CommandContext(ctx, cliPath, cfg.Args...)
	cmd.Env = env
	if cfg.WorkingDirectory != "" {
		cmd.Dir = cfg.WorkingDirectory
	}

	// The pipe is made here rather than by cmd.StdinPipe so the write end is an *os.File whose
	// write deadline Send can use to interrupt a blocked write and learn how much of it landed.
	stdinR, stdin, err := os.Pipe()
	if err != nil {
		return nil, &sdkerrors.CLIConnectionError{Msg: "failed to create stdin pipe: " + err.Error()}
	}
	cmd.Stdin = stdinR

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdinR.Close()
		stdin.Close()
		return nil, &sdkerrors.CLIConnectionError{Msg: "failed to create stdout pipe: " + err.Error()}
	}

	if err := cmd.Start(); err != nil {
		stdinR.Close()
		stdin.Close()
		return nil, &sdkerrors.CLIConnectionError{Msg: "failed to start claude: " + err.Error()}
	}
	stdinR.Close()

	t := &Transport{
		cmd:    cmd,
		stdin:  stdin,
		reader: jsonlines.NewReader(stdout),
		lines:  make(chan []byte),
		done:   make(chan struct{}),
	}
	go t.pump()
	return t, nil
}

// Send writes data to the subprocess stdin as a JSON line.
//
// Send returns ctx.Err() as soon as ctx is done. If nothing of the line had reached the process
// by then (the pipe was full, or an earlier Send still held the write), the line is abandoned and
// the transport stays usable. If the write was already under way it may have reached the process
// in part, so the stream can no longer be trusted: stdin is closed and later calls to Send fail.
// It returns io.ErrClosedPipe once the transport is closed.
func (t *Transport) Send(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-t.done:
		return io.ErrClosedPipe
	default:
	}

	line := make([]byte, len(data)+1)
	copy(line, data)
	line[len(data)] = '\n'

	type result struct {
		n   int
		err error
	}
	const (
		pending = iota
		writing
		finished
		abandoned
	)
	var (
		stateMu sync.Mutex
		state   = pending
	)
	res := make(chan result, 1)
	go func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		stateMu.Lock()
		if state == abandoned {
			stateMu.Unlock()
			res <- result{}
			return
		}
		state = writing
		stateMu.Unlock()
		n, err := t.stdin.Write(line)
		stateMu.Lock()
		state = finished
		stateMu.Unlock()
		// A deadline set by a cancel that lost the race must not outlive this write.
		_ = t.stdin.SetWriteDeadline(time.Time{})
		res <- result{n, err}
	}()

	select {
	case r := <-res:
		return r.err
	case <-t.done:
		return io.ErrClosedPipe
	case <-ctx.Done():
	}

	stateMu.Lock()
	switch state {
	case pending:
		state = abandoned
		stateMu.Unlock()
		return ctx.Err()
	case writing:
		if t.stdin.SetWriteDeadline(time.Unix(1, 0)) != nil {
			// No deadlines on this platform's pipe: the only way to release the write is to close it.
			stateMu.Unlock()
			t.closeStdin()
			return ctx.Err()
		}
	}
	stateMu.Unlock()

	r := <-res
	switch {
	case r.err == nil && r.n == len(line):
		// The write finished before the cancel took effect: the line went through whole.
		return nil
	case r.n == 0 && errors.Is(r.err, os.ErrDeadlineExceeded):
		return ctx.Err()
	}
	t.closeStdin()
	return ctx.Err()
}

func (t *Transport) closeStdin() {
	t.stdinOnce.Do(func() { _ = t.stdin.Close() })
}

// pump reads stdout on its own goroutine so Receive can select on a context. A line is only
// read from the pipe, never dropped: it waits on the unbuffered channel until a Receive takes it.
func (t *Transport) pump() {
	defer close(t.lines)
	for {
		line, err := t.reader.ReadLine()
		if err != nil {
			t.readErr = err
			return
		}
		select {
		case t.lines <- line:
		case <-t.done:
			t.readErr = io.EOF
			return
		}
	}
}

// Receive returns the next line from the subprocess stdout. It returns ctx.Err() as soon as ctx is
// done, without consuming a line, so a later Receive still sees every line. It returns io.EOF when
// the stream ends or the transport is closed, and keeps returning it.
func (t *Transport) Receive(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case line, ok := <-t.lines:
		if !ok {
			return nil, t.readErr
		}
		return line, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.done:
		return nil, io.EOF
	}
}

// Close shuts down the subprocess gracefully: close stdin → wait 5s → SIGTERM → wait 5s → SIGKILL.
func (t *Transport) Close() error {
	var closeErr error
	t.once.Do(func() {
		close(t.done)

		// Close stdin to signal EOF to the subprocess
		t.closeStdin()

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

// checkVersion runs claude -v. A version below MinimumCLIVersion is logged, or returned as a
// *CLIVersionError when enforce is set. An unreadable version is never an error.
func checkVersion(cliPath string, enforce bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, cliPath, "-v").Output()
	if err != nil {
		return nil
	}

	m := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`).FindStringSubmatch(string(out))
	if m == nil {
		return nil
	}
	var v, min [3]int
	for i := range v {
		fmt.Sscanf(m[i+1], "%d", &v[i])
	}
	fmt.Sscanf(MinimumCLIVersion, "%d.%d.%d", &min[0], &min[1], &min[2])

	if v[0] > min[0] || (v[0] == min[0] && (v[1] > min[1] || (v[1] == min[1] && v[2] >= min[2]))) {
		return nil
	}
	found := fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
	if enforce {
		return &sdkerrors.CLIVersionError{CLIPath: cliPath, Found: found, Required: MinimumCLIVersion}
	}
	log.Printf("WARNING: Claude Code version %s is below minimum required %s. Some features may not work correctly.",
		found, MinimumCLIVersion)
	return nil
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
