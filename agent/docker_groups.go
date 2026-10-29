package agent

import (
	"strings"
)

// Docker labels that say which stack and service a container belongs to.
const (
	labelSwarmStack     = "com.docker.stack.namespace"
	labelSwarmService   = "com.docker.swarm.service.name"
	labelComposeProject = "com.docker.compose.project"
	labelComposeService = "com.docker.compose.service"
)

// maxGroupValueLength caps a stack or service name. Labels are set by whoever runs the container
// and Docker puts no limit on their length, while these two values are stored on every container
// record and repeated in every stats record, so an oversized label must not be able to grow the
// history without bound. Real stack and service names are far below this.
const maxGroupValueLength = 100

// groupLabels holds the only labels grouping needs. Decoding into a struct rather than a map skips
// every other label without allocating, which matters because a busy node reports dozens of labels
// per container and this runs on every poll.
type groupLabels struct {
	SwarmStack     string `json:"com.docker.stack.namespace"`
	SwarmService   string `json:"com.docker.swarm.service.name"`
	ComposeProject string `json:"com.docker.compose.project"`
	ComposeService string `json:"com.docker.compose.service"`
}

// group returns the stack and the service of a container.
//
// Swarm wins over Compose when both sets are present: a stack deployed with `docker stack deploy`
// carries both, and there the Compose labels describe the source file, not the running stack. A
// label that is present but empty or whitespace counts as missing.
//
// Replicas of one service on a node deliberately get the same pair. This is the opposite of
// resolveContainerNames, which reverts colliding names so that series are never merged; here
// Stats.Name and Stats.Id are what keep the replicas apart.
func (l groupLabels) group() (stack, service string) {
	stack = firstNonEmpty(l.SwarmStack, l.ComposeProject)
	service = trimStackPrefix(stack, firstNonEmpty(l.SwarmService, l.ComposeService))
	return truncateGroupValue(stack), truncateGroupValue(service)
}

// truncateGroupValue cuts a value to maxGroupValueLength characters, on a rune boundary so the
// result stays valid UTF-8.
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

// groupLabelsFromMap reads the same labels out of a full label map, which the agent decodes only
// when CONTAINER_NAME_LABEL asks for one.
func groupLabelsFromMap(labels map[string]string) groupLabels {
	return groupLabels{
		SwarmStack:     labels[labelSwarmStack],
		SwarmService:   labels[labelSwarmService],
		ComposeProject: labels[labelComposeProject],
		ComposeService: labels[labelComposeService],
	}
}

// firstNonEmpty returns the first value that holds more than whitespace, trimmed.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// trimStackPrefix drops the stack prefix Swarm puts on service names (mentors_web -> web), so the
// stack is not repeated in every row. The name is kept when trimming would empty it.
func trimStackPrefix(stack, service string) string {
	if stack == "" {
		return service
	}
	if rest, ok := strings.CutPrefix(service, stack+"_"); ok && rest != "" {
		return rest
	}
	return service
}
