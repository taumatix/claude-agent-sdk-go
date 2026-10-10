package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SandboxSettings configures how the CLI sandboxes Bash commands (macOS and Linux). It is sent
// as the "sandbox" key of the --settings value. Every field is optional: a nil pointer or an empty
// slice is left out so the CLI's own default applies, which is how "false" is told apart from "unset".
//
// Filesystem and network limits for the tools themselves are permission rules, not sandbox
// settings; Network here configures the sandbox's own isolation of Bash commands.
type SandboxSettings struct {
	Enabled                   *bool                    `json:"enabled,omitempty"`
	AutoAllowBashIfSandboxed  *bool                    `json:"autoAllowBashIfSandboxed,omitempty"`
	ExcludedCommands          []string                 `json:"excludedCommands,omitempty"`
	AllowUnsandboxedCommands  *bool                    `json:"allowUnsandboxedCommands,omitempty"`
	Network                   *SandboxNetworkConfig    `json:"network,omitempty"`
	IgnoreViolations          *SandboxIgnoreViolations `json:"ignoreViolations,omitempty"`
	EnableWeakerNestedSandbox *bool                    `json:"enableWeakerNestedSandbox,omitempty"`
}

// SandboxNetworkConfig is SandboxSettings.Network.
type SandboxNetworkConfig struct {
	AllowedDomains          []string `json:"allowedDomains,omitempty"`
	DeniedDomains           []string `json:"deniedDomains,omitempty"`
	AllowManagedDomainsOnly *bool    `json:"allowManagedDomainsOnly,omitempty"`
	AllowUnixSockets        []string `json:"allowUnixSockets,omitempty"`
	AllowAllUnixSockets     *bool    `json:"allowAllUnixSockets,omitempty"`
	AllowLocalBinding       *bool    `json:"allowLocalBinding,omitempty"`
	AllowMachLookup         []string `json:"allowMachLookup,omitempty"`
	HTTPProxyPort           *int     `json:"httpProxyPort,omitempty"`
	SocksProxyPort          *int     `json:"socksProxyPort,omitempty"`
}

// SandboxIgnoreViolations lists violations the sandbox should not report.
type SandboxIgnoreViolations struct {
	File    []string `json:"file,omitempty"`
	Network []string `json:"network,omitempty"`
}

// mergedSettings is the value of --settings: opts.Settings untouched when there is no sandbox,
// otherwise a JSON object holding opts.Settings (a JSON object or the path of a JSON file) with
// the sandbox key set. A Settings value that cannot be read or parsed is an error rather than
// silently dropped, since the caller's settings would otherwise vanish.
func mergedSettings(opts Options) (string, error) {
	if opts.Sandbox == nil {
		return opts.Settings, nil
	}
	obj := map[string]json.RawMessage{}
	if s := strings.TrimSpace(opts.Settings); s != "" {
		raw := []byte(s)
		if !strings.HasPrefix(s, "{") {
			b, err := os.ReadFile(s)
			if err != nil {
				return "", fmt.Errorf("reading Options.Settings to merge the sandbox into it: %w", err)
			}
			raw = b
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return "", fmt.Errorf("Options.Settings is not a JSON object, so the sandbox cannot be merged into it: %w", err)
		}
	}
	sb, err := json.Marshal(opts.Sandbox)
	if err != nil {
		return "", err
	}
	obj["sandbox"] = sb
	out, err := json.Marshal(obj)
	return string(out), err
}

// validateOptions reports Options that cannot be turned into a CLI invocation, before one starts.
func validateOptions(opts Options) error {
	if _, err := sdkServers(opts); err != nil {
		return err
	}
	_, err := mergedSettings(opts)
	return err
}
