package agent

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

// CanUseToolShadowedWarning returns the advisory upstream emits when a
// permission callback is set alongside options that auto-approve tool calls
// before the callback is consulted, or "" when there is nothing to say. It is
// the Go counterpart of upstream's CanUseToolShadowedWarning, and is derived
// from upstream's source, not from a live CLI: allow rules in settings files
// can shadow the callback too but are not visible here.
//
// The SDK logs it (once per distinct message per process) when a session
// starts; call this to get the text without starting one.
func CanUseToolShadowedWarning(opts Options) string {
	if opts.ToolPermissionFunc == nil && opts.ToolPermissionHandler == nil {
		return ""
	}
	if opts.PermissionMode == PermissionModeBypass {
		return "can_use_tool will not be invoked: permission_mode " +
			"'bypassPermissions' auto-approves every tool call (except " +
			"explicit deny rules) before the callback is consulted. To gate " +
			"every tool call, use a PreToolUse hook instead."
	}
	var shadowed []string
	seen := map[string]bool{}
	for _, entry := range opts.AllowedTools {
		if tool, ok := wholeToolAllowed(entry); ok && !seen[tool] {
			seen[tool] = true
			shadowed = append(shadowed, tool)
		}
	}
	if len(shadowed) == 0 {
		return ""
	}
	return fmt.Sprintf("can_use_tool will not be invoked for: %s. "+
		"An allowed_tools entry that allows a whole tool auto-approves it "+
		"before the callback is consulted. To gate every tool call, use a "+
		"PreToolUse hook; or narrow the entry so calls fall through to "+
		"can_use_tool. Allow rules from settings files can also shadow the "+
		"callback but are not visible here.", strings.Join(shadowed, ", "))
}

// wholeToolAllowed mirrors the CLI's rule parser as upstream ports it: an entry
// allows a whole tool when it has no "(...)" specifier, or an empty or lone-"*"
// one. A real specifier ("Bash(ls:*)") only allows matching invocations, and a
// malformed entry matches nothing.
func wholeToolAllowed(entry string) (string, bool) {
	if strings.TrimSpace(entry) == "" {
		return "", false
	}
	open := strings.Index(entry, "(")
	if open == -1 {
		return entry, true
	}
	if open == 0 || !strings.HasSuffix(entry, ")") {
		return "", false
	}
	if inner := entry[open+1 : len(entry)-1]; inner == "" || inner == "*" {
		return entry[:open], true
	}
	return "", false
}

var shadowWarned sync.Map

// warnIfToolPermissionShadowed logs the advisory, once per distinct message per
// process (upstream's default warning filter does the same).
func warnIfToolPermissionShadowed(opts Options) {
	msg := CanUseToolShadowedWarning(opts)
	if msg == "" {
		return
	}
	if _, dup := shadowWarned.LoadOrStore(msg, true); !dup {
		log.Printf("WARNING: %s", msg)
	}
}
