package messages

// PermissionDeniedMessage reports a tool call the CLI refused without an
// interactive prompt: a deny rule, dontAsk mode, the auto-mode classifier, or
// an "ask" with no permission callback to ask (a Query without CanUseTool).
// A refusal through the SDK's own CanUseTool callback is not reported here;
// the callback made it.
//
// The CLI calls it best-effort advisory: in rare races a denial happens with
// no frame. ResultMessage's permission denials are the complete list.
type PermissionDeniedMessage struct {
	ToolName  string
	ToolUseID string
	// AgentID is the subagent the call came from, empty for the main agent.
	AgentID string
	// ReasonType says which component decided, such as "rule", "mode",
	// "classifier", "asyncAgent" or "subcommandResults" (a deny rule matching
	// part of a Bash command, as seen live).
	ReasonType string
	// ReasonCode is set only for reasons a host can act on, from a closed set
	// the CLI extends: "outside_reads_blocked", "memory_paused",
	// "classifier_transcript_too_long". The CLI marks it internal.
	ReasonCode string
	// Reason is the deciding component's own explanation, when it gave one.
	Reason string
	// Message is what the model was told in the tool result.
	Message   string
	UUID      string
	SessionID string
}
