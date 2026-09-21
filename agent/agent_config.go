package agent

import "github.com/henrygd/beszel/internal/entities/agentconfig"

// applyAgentConfig applies settings pushed from the hub. Settings that are also
// available as env vars are ignored by their owners when the env var is set.
func (a *Agent) applyAgentConfig(cfg agentconfig.Config) {
	if a.dockerManager != nil {
		a.dockerManager.setHubExcludeContainers(cfg.ExcludeContainers)
	}
	a.setHubNics(cfg.Nics)
	if a.systemdManager != nil {
		a.systemdManager.setHubServicePatterns(cfg.ServicePatterns)
	}
}
