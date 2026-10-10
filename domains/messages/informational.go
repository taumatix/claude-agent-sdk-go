package messages

// InformationalMessage is a notice the CLI wants shown to the user.
type InformationalMessage struct {
	Content string
	// Level is the CLI's severity word, passed through; empty when the CLI
	// sends none. The values it uses are not enumerated here because none has
	// been captured live.
	Level string
	// PreventContinuation is true when the CLI says the turn should not go on
	// after this notice.
	PreventContinuation bool
	UUID                string
	SessionID           string
}
