package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
)

func TestOptionValueConstantsAreAssignableAndReachTheCLI(t *testing.T) {
	args := agent.BuildCLIArgs(agent.Options{
		Effort:          agent.EffortXHigh,
		Thinking:        agent.ThinkingAdaptive,
		ThinkingDisplay: agent.ThinkingDisplayOmitted,
		SettingSources:  []string{agent.SettingSourceUser, agent.SettingSourceLocal},
		Betas:           []string{agent.Beta1MContext},
	})
	for _, want := range []string{"xhigh", "adaptive", "omitted", "context-1m-2025-08-07"} {
		assert.Contains(t, args, want)
	}
}
