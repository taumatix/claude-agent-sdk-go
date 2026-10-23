package messages

import "github.com/taumatix/claude-agent-sdk-go/domains/protocol"

// SystemInitMCPServer is an MCP server listed on the init message.
type SystemInitMCPServer struct {
	Name   string
	Status string
	// Source is where the definition came from ("sdk", "plugin", or a config
	// scope such as "user"); empty on CLIs that predate the field.
	Source string
}

// SystemInitPlugin is a plugin loaded for the session.
type SystemInitPlugin struct {
	Name string
	Path string
	// Version is the manifest's version verbatim; empty when it declares none.
	Version string
}

// SystemInitPluginError is a plugin that failed to load.
type SystemInitPluginError struct {
	Plugin string
	// Type is a failure category from an open set; treat unknown values as a
	// generic failure.
	Type    string
	Message string
	// Path is set only when a directory entry did not load at all.
	Path string
}

// SystemInitMessage is the session metadata a CLI sends as its first system
// message of a turn. Field names follow upstream's SystemInitData. An absent key
// leaves the zero value, so an empty Capabilities means "not reported" (CLIs
// before 2.1.205), not "supports nothing".
type SystemInitMessage struct {
	UUID                   string
	SessionID              string
	Agents                 []string
	APIKeySource           string
	Betas                  []string
	ClaudeCodeVersion      string
	Cwd                    string
	Tools                  []string
	MCPServers             []SystemInitMCPServer
	Model                  string
	PermissionMode         string
	SlashCommands          []string
	TerminalSlashCommands  []string
	OutputStyle            string
	Skills                 []string
	Plugins                []SystemInitPlugin
	PluginErrors           []SystemInitPluginError
	FastModeState          string
	FastModeDisabledReason string
	// Capabilities is an open set; ignore values you do not recognise.
	Capabilities []string
}

func initFromWire(p protocol.InitPayload) *SystemInitMessage {
	m := &SystemInitMessage{
		UUID: p.UUID, SessionID: p.SessionID, Agents: p.Agents, APIKeySource: p.APIKeySource,
		Betas: p.Betas, ClaudeCodeVersion: p.ClaudeCodeVersion, Cwd: p.Cwd, Tools: p.Tools,
		Model: p.Model, PermissionMode: p.PermissionMode, SlashCommands: p.SlashCommands,
		TerminalSlashCommands: p.TerminalSlashCommands, OutputStyle: p.OutputStyle, Skills: p.Skills,
		FastModeState: p.FastModeState, FastModeDisabledReason: p.FastModeDisabledReason,
		Capabilities: p.Capabilities,
	}
	for _, s := range p.MCPServers {
		m.MCPServers = append(m.MCPServers, SystemInitMCPServer(s))
	}
	for _, pl := range p.Plugins {
		m.Plugins = append(m.Plugins, SystemInitPlugin(pl))
	}
	for _, e := range p.PluginErrors {
		m.PluginErrors = append(m.PluginErrors, SystemInitPluginError(e))
	}
	return m
}
