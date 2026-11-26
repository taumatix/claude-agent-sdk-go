package agent

// Values the string-valued Options fields are documented to take. The fields
// stay string for good, and these are untyped constants, so a value a newer CLI
// accepts can be assigned without waiting for a release, and retyping the field
// can never become a breaking change. The lists come from upstream's SDK source
// and the CLI's --help; none is checked against a live run.
const (
	// Options.Effort
	EffortLow    = "low"
	EffortMedium = "medium"
	EffortHigh   = "high"
	EffortXHigh  = "xhigh"
	EffortMax    = "max"

	// Options.Thinking
	ThinkingAdaptive = "adaptive"
	ThinkingDisabled = "disabled"

	// Options.ThinkingDisplay
	ThinkingDisplaySummarized = "summarized"
	ThinkingDisplayOmitted    = "omitted"

	// Options.SettingSources entries
	SettingSourceUser    = "user"
	SettingSourceProject = "project"
	SettingSourceLocal   = "local"

	// Options.Betas entries upstream lists
	Beta1MContext = "context-1m-2025-08-07"
)
