package common

type WebSocketAction = uint8

// Not implemented yet
// type AgentError = uint8

const (
	// Request system data from agent
	GetData WebSocketAction = iota
	// Check the fingerprint of the agent
	CheckFingerprint
	// Push configuration update to agent
	ConfigUpdate
)

// HubRequest defines the structure for requests sent from hub to agent.
type HubRequest[T any] struct {
	Action WebSocketAction `cbor:"0,keyasint"`
	Data   T               `cbor:"1,keyasint,omitempty,omitzero"`
	// Error  AgentError      `cbor:"error,omitempty,omitzero"`
}

type FingerprintRequest struct {
	Signature   []byte `cbor:"0,keyasint"`
	NeedSysInfo bool   `cbor:"1,keyasint"` // For universal token system creation
}

type FingerprintResponse struct {
	Fingerprint string `cbor:"0,keyasint"`
	// Optional system info for universal token system creation
	Hostname string `cbor:"1,keyasint,omitempty,omitzero"`
	Port     string `cbor:"2,keyasint,omitempty,omitzero"`
}

// ConfigUpdateRequest contains configuration data to push to agent
type ConfigUpdateRequest struct {
	LogLevel      string            `cbor:"0,keyasint,omitempty,omitzero"`
	MemCalc       string            `cbor:"1,keyasint,omitempty,omitzero"`
	ExtraFs       []string          `cbor:"2,keyasint,omitempty,omitzero"`
	DataDir       string            `cbor:"3,keyasint,omitempty,omitzero"`
	DockerHost    string            `cbor:"4,keyasint,omitempty,omitzero"`
	Filesystem    string            `cbor:"5,keyasint,omitempty,omitzero"`
	Listen        string            `cbor:"6,keyasint,omitempty,omitzero"`
	Network       string            `cbor:"7,keyasint,omitempty,omitzero"`
	Nics          string            `cbor:"8,keyasint,omitempty,omitzero"`
	PrimarySensor string            `cbor:"9,keyasint,omitempty,omitzero"`
	Sensors       string            `cbor:"10,keyasint,omitempty,omitzero"`
	SysSensors    string            `cbor:"11,keyasint,omitempty,omitzero"`
	Environment   map[string]string `cbor:"12,keyasint,omitempty,omitzero"`
	Version       uint64            `cbor:"13,keyasint"`
	ForceRestart  bool              `cbor:"14,keyasint,omitempty,omitzero"`
}

// ConfigUpdateResponse confirms configuration update
type ConfigUpdateResponse struct {
	Success       bool   `cbor:"0,keyasint"`
	Version       uint64 `cbor:"1,keyasint"`
	Error         string `cbor:"2,keyasint,omitempty,omitzero"`
	RestartNeeded bool   `cbor:"3,keyasint,omitempty,omitzero"`
}
