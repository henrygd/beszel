// Package agentconfig defines per-system agent settings managed by the hub.
package agentconfig

// Config holds agent settings pushed from hub to agent as CBOR. The hub builds
// it from the system's system_config record, which has one column per setting.
// A sync always replaces the full config, so an empty Config clears all
// hub-provided settings.
type Config struct {
	// ExcludeContainers is a list of container name glob patterns to skip.
	// Ignored by the agent if the EXCLUDE_CONTAINERS env var is set.
	ExcludeContainers []string `cbor:"0,keyasint,omitempty"`
	// ServicePatterns is a list of systemd unit name patterns to monitor. Empty
	// means the agent default. Ignored by the agent if the SERVICE_PATTERNS env var is set.
	ServicePatterns []string `cbor:"1,keyasint,omitempty"`
	// Nics is a network interface list in NICS env var syntax: a leading '-' excludes
	// the listed interfaces instead of monitoring only those. Empty means automatic
	// detection. Ignored by the agent if the NICS env var is set.
	Nics string `cbor:"2,keyasint,omitempty"`
}
