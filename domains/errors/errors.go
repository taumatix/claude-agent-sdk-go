// Package errors defines all SDK error types.
package errors

import "fmt"

// CLINotFoundError is returned when the Claude Code CLI binary cannot be found.
type CLINotFoundError struct {
	Msg     string
	CLIPath string
}

func (e *CLINotFoundError) Error() string {
	if e.CLIPath != "" {
		return fmt.Sprintf("%s: %s", e.Msg, e.CLIPath)
	}
	return e.Msg
}

// CLIVersionError is returned when the CLI is older than the version this SDK supports and the
// caller asked for that to be enforced (Options.RequireMinimumCLIVersion).
type CLIVersionError struct {
	CLIPath  string
	Found    string
	Required string
}

func (e *CLIVersionError) Error() string {
	return fmt.Sprintf("Claude Code %s at %s is older than the minimum supported %s; upgrade it with "+
		"npm install -g @anthropic-ai/claude-code", e.Found, e.CLIPath, e.Required)
}

// CLIConnectionError is returned when a connection to the CLI subprocess fails.
type CLIConnectionError struct {
	Msg string
}

func (e *CLIConnectionError) Error() string {
	return e.Msg
}

// ProcessError is returned when the CLI subprocess exits with a non-zero code
// or encounters a fatal error during execution.
type ProcessError struct {
	Msg      string
	ExitCode int
	Stderr   string
}

func (e *ProcessError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("%s (exit code: %d): %s", e.Msg, e.ExitCode, e.Stderr)
	}
	return fmt.Sprintf("%s (exit code: %d)", e.Msg, e.ExitCode)
}

// CLIJSONDecodeError is returned when a line of output from the CLI cannot be
// decoded as JSON.
type CLIJSONDecodeError struct {
	Msg           string
	Line          []byte
	OriginalError error
}

func (e *CLIJSONDecodeError) Error() string {
	return fmt.Sprintf("%s: %v", e.Msg, e.OriginalError)
}

// Unwrap returns the underlying JSON decode error.
func (e *CLIJSONDecodeError) Unwrap() error {
	return e.OriginalError
}

// MessageParseError is returned when a parsed wire message cannot be converted
// to a structured SDK message.
type MessageParseError struct {
	Msg  string
	Data map[string]any
}

func (e *MessageParseError) Error() string {
	return e.Msg
}

// InitializeError is returned when the SDK-CLI initialization handshake fails.
type InitializeError struct {
	Msg string
}

func (e *InitializeError) Error() string {
	return e.Msg
}
