package agent

import (
	"strings"
)

const (
	labelSwarmStack     = "com.docker.stack.namespace"
	labelSwarmService   = "com.docker.swarm.service.name"
	labelComposeProject = "com.docker.compose.project"
	labelComposeService = "com.docker.compose.service"
)

// Labels carry no length limit of their own, and these two values are stored on every container.
const maxGroupValueLength = 100

// Decoding into a struct rather than a map skips every other label without allocating.
type groupLabels struct {
	SwarmStack     string `json:"com.docker.stack.namespace"`
	SwarmService   string `json:"com.docker.swarm.service.name"`
	ComposeProject string `json:"com.docker.compose.project"`
	ComposeService string `json:"com.docker.compose.service"`
}

// Swarm wins over Compose: a stack deployed with `docker stack deploy` carries both, and there the
// Compose labels describe the source file, not the running stack. Replicas on a node deliberately
// share one pair; Stats.Name and Stats.Id are what keep them apart.
func (l groupLabels) group() (stack, service string) {
	stack = firstNonEmpty(l.SwarmStack, l.ComposeProject)
	service = trimStackPrefix(stack, firstNonEmpty(l.SwarmService, l.ComposeService))
	return truncateGroupValue(stack), truncateGroupValue(service)
}

// Cuts on a rune boundary so the result stays valid UTF-8.
func truncateGroupValue(value string) string {
	if len(value) <= maxGroupValueLength {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxGroupValueLength {
		return value
	}
	return string(runes[:maxGroupValueLength])
}

// Used when CONTAINER_NAME_LABEL has already made the agent decode the full label map.
func groupLabelsFromMap(labels map[string]string) groupLabels {
	return groupLabels{
		SwarmStack:     labels[labelSwarmStack],
		SwarmService:   labels[labelSwarmService],
		ComposeProject: labels[labelComposeProject],
		ComposeService: labels[labelComposeService],
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// Swarm prefixes service names with the stack (mentors_web -> web); kept when trimming empties it.
func trimStackPrefix(stack, service string) string {
	if stack == "" {
		return service
	}
	if rest, ok := strings.CutPrefix(service, stack+"_"); ok && rest != "" {
		return rest
	}
	return service
}
