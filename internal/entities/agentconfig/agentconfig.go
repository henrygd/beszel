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
}
