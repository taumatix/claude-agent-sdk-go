package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/taumatix/claude-agent-sdk-go/domains/transport"
)

// PermissionMode controls what tools the CLI is allowed to use without asking.
type PermissionMode string

const (
	PermissionModeDefault     PermissionMode = "default"
	PermissionModeAcceptEdits PermissionMode = "acceptEdits"
	PermissionModePlan        PermissionMode = "plan"
	PermissionModeBypass      PermissionMode = "bypassPermissions"
)

// AgentDefinition is a custom subagent passed to the CLI with --agents. The CLI's help documents
// exactly these two fields; others it may accept are not modelled until checked.
type AgentDefinition struct {
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

// MCPServerConfig describes a single MCP server configuration.
type MCPServerConfig struct {
	Name    string
	Type    string
	Command string
	Args    []string
	Env     map[string]string
	URL     string
}

// Options configures a Query or Client session.
type Options struct {
	// Session control
	SessionID       string
	ContinueSession bool
	ResumeSessionID string
	ForkSession     bool

	// Model selection
	Model         string
	FallbackModel string

	// Conversation limits
	MaxTurns     *int
	MaxBudgetUSD *float64

	// Prompting
	SystemPrompt       string
	AppendSystemPrompt string

	// Tool control
	AllowedTools    []string
	DisallowedTools []string
	Tools           []string

	// Permission mode
	PermissionMode PermissionMode

	// Working directory
	WorkingDirectory string
	AddDirs          []string

	// MCP
	MCPServers []MCPServerConfig

	// Streaming
	IncludePartialMessages bool

	// Settings
	SettingSources []string
	Settings       string

	// Beta features
	Betas []string

	// Thinking / effort
	MaxThinkingTokens *int
	Effort            string

	// JSONSchema is a JSON Schema the CLI validates the final answer against (--json-schema).
	JSONSchema string

	// NoSessionPersistence keeps the session off disk, so it cannot be resumed
	// (--no-session-persistence).
	NoSessionPersistence bool

	// StrictMCPConfig ignores every MCP configuration except MCPServers (--strict-mcp-config).
	StrictMCPConfig bool

	// PluginDirs load a plugin directory or .zip for this session only (--plugin-dir, repeated).
	PluginDirs []string

	// Agents defines custom subagents by name (--agents).
	Agents map[string]AgentDefinition

	// DisableSlashCommands disables all skills (--disable-slash-commands).
	DisableSlashCommands bool

	// PluginURLs fetch a plugin .zip for this session only (--plugin-url, repeated).
	PluginURLs []string

	// Worktree runs the session in a new git worktree (--worktree); WorktreeName
	// names it, and is ignored unless Worktree is set.
	Worktree     bool
	WorktreeName string

	// FromPR resumes the session linked to a pull request, by number or URL
	// (--from-pr). Empty is not sent: the bare flag opens an interactive picker.
	FromPR string

	// PermissionPromptTool is the MCP tool that answers permission prompts
	// (--permission-prompt-tool).
	PermissionPromptTool string

	// PermissionPrompts says who answers permission prompts under --print: "host"
	// or "none" (--permission-prompts).
	PermissionPrompts string

	// IncludeHookEvents streams the hook lifecycle events (--include-hook-events).
	IncludeHookEvents bool

	// Extra arbitrary CLI flags (key → value, nil value = boolean flag)
	ExtraArgs map[string]string

	// Callbacks
	ToolPermissionHandler ToolPermissionHandler
	HookHandlers          map[string][]HookMatcher // event name → matchers

	// Transport override (nil = spawn subprocess)
	Transport transport.Transport

	// CLI path override (empty = auto-detect)
	CLIPath string

	// Environment variables to pass to the subprocess
	Env map[string]string

	// RequireMinimumCLIVersion makes a CLI older than the supported minimum an error
	// (*errors.CLIVersionError) rather than a logged warning. Off by default so existing callers
	// keep working; a future release will turn it on.
	RequireMinimumCLIVersion bool
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		ExtraArgs: map[string]string{},
		Env:       map[string]string{},
	}
}

// BuildCLIArgs converts Options to a list of CLI flag arguments suitable for passing
// to exec.Command as args. Includes --output-format, all option-specific flags,
// and --input-format stream-json at the end.
func BuildCLIArgs(opts Options) []string {
	args := []string{"--output-format", "stream-json", "--verbose"}

	// System prompt — always set (empty string clears it)
	if opts.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.AppendSystemPrompt)
	} else {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}

	if len(opts.Tools) > 0 {
		args = append(args, "--tools", strings.Join(opts.Tools, ","))
	}

	if len(opts.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}

	if len(opts.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}

	if opts.MaxTurns != nil {
		args = append(args, "--max-turns", strconv.Itoa(*opts.MaxTurns))
	}

	if opts.MaxBudgetUSD != nil {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(*opts.MaxBudgetUSD, 'f', -1, 64))
	}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}

	if len(opts.Betas) > 0 {
		args = append(args, "--betas", strings.Join(opts.Betas, ","))
	}

	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", string(opts.PermissionMode))
	}

	if opts.ContinueSession {
		args = append(args, "--continue")
	}

	if opts.ResumeSessionID != "" {
		args = append(args, "--resume", opts.ResumeSessionID)
	}

	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}

	if opts.Settings != "" {
		args = append(args, "--settings", opts.Settings)
	}

	for _, dir := range opts.AddDirs {
		args = append(args, "--add-dir", dir)
	}

	if len(opts.MCPServers) > 0 {
		servers := make(map[string]interface{}, len(opts.MCPServers))
		for _, s := range opts.MCPServers {
			cfg := map[string]interface{}{}
			if s.Type != "" {
				cfg["type"] = s.Type
			}
			if s.Command != "" {
				cfg["command"] = s.Command
			}
			if len(s.Args) > 0 {
				cfg["args"] = s.Args
			}
			if len(s.Env) > 0 {
				cfg["env"] = s.Env
			}
			if s.URL != "" {
				cfg["url"] = s.URL
			}
			servers[s.Name] = cfg
		}
		mcpJSON, err := json.Marshal(map[string]interface{}{"mcpServers": servers})
		if err == nil {
			args = append(args, "--mcp-config", string(mcpJSON))
		}
	}

	if opts.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}

	if opts.ForkSession {
		args = append(args, "--fork-session")
	}

	if len(opts.SettingSources) > 0 {
		args = append(args, "--setting-sources", strings.Join(opts.SettingSources, ","))
	}

	if opts.MaxThinkingTokens != nil {
		args = append(args, "--max-thinking-tokens", strconv.Itoa(*opts.MaxThinkingTokens))
	}

	if opts.Effort != "" {
		args = append(args, "--effort", opts.Effort)
	}

	if opts.JSONSchema != "" {
		args = append(args, "--json-schema", opts.JSONSchema)
	}
	if opts.NoSessionPersistence {
		args = append(args, "--no-session-persistence")
	}
	if opts.StrictMCPConfig {
		args = append(args, "--strict-mcp-config")
	}
	for _, dir := range opts.PluginDirs {
		args = append(args, "--plugin-dir", dir)
	}
	if len(opts.Agents) > 0 {
		// A map marshals with sorted keys, so the argument is the same on every run.
		if agentsJSON, err := json.Marshal(opts.Agents); err == nil {
			args = append(args, "--agents", string(agentsJSON))
		}
	}
	if opts.DisableSlashCommands {
		args = append(args, "--disable-slash-commands")
	}
	for _, u := range opts.PluginURLs {
		args = append(args, "--plugin-url", u)
	}
	if opts.IncludeHookEvents {
		args = append(args, "--include-hook-events")
	}
	if opts.Worktree {
		args = append(args, "--worktree")
		if opts.WorktreeName != "" {
			args = append(args, opts.WorktreeName)
		}
	}
	if opts.FromPR != "" {
		args = append(args, "--from-pr", opts.FromPR)
	}
	if opts.PermissionPromptTool != "" {
		args = append(args, "--permission-prompt-tool", opts.PermissionPromptTool)
	}
	if opts.PermissionPrompts != "" {
		args = append(args, "--permission-prompts", opts.PermissionPrompts)
	}

	// Extra arbitrary flags
	for k, v := range opts.ExtraArgs {
		if v == "" {
			args = append(args, fmt.Sprintf("--%s", k))
		} else {
			args = append(args, fmt.Sprintf("--%s", k), v)
		}
	}

	// Always last
	args = append(args, "--input-format", "stream-json")

	return args
}
