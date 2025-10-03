package common

import (
	"github.com/henrygd/beszel/internal/entities/system"
)

type WebSocketAction = uint8

const (
	// Request system data from agent
	GetData WebSocketAction = iota
	// Check the fingerprint of the agent
	CheckFingerprint
	// Add new actions here...
)

// HubRequest defines the structure for requests sent from hub to agent.
type HubRequest[T any] struct {
	Action WebSocketAction `cbor:"0,keyasint"`
	Data   T               `cbor:"1,keyasint,omitempty,omitzero"`
	Id     *uint32         `cbor:"2,keyasint,omitempty"`
}

// AgentResponse defines the structure for responses sent from agent to hub.
type AgentResponse struct {
	Id          *uint32              `cbor:"0,keyasint,omitempty"`
	SystemData  *system.CombinedData `cbor:"1,keyasint,omitempty,omitzero"`
	Fingerprint *FingerprintResponse `cbor:"2,keyasint,omitempty,omitzero"`
	Error       string               `cbor:"3,keyasint,omitempty,omitzero"`
	// RawBytes    []byte               `cbor:"4,keyasint,omitempty,omitzero"`
}

type FingerprintRequest struct {
	Signature   []byte `cbor:"0,keyasint"`
	NeedSysInfo bool   `cbor:"1,keyasint"` // For universal token system creation
}

type FingerprintResponse struct {
	Fingerprint string `cbor:"0,keyasint"`
	// Optional system info for universal token system creation
	Hostname string `cbor:"1,keyasint,omitzero"`
	Port     string `cbor:"2,keyasint,omitzero"`
	Name     string `cbor:"3,keyasint,omitzero"`
}

type DataRequestOptions struct {
	CacheTimeMs uint16 `cbor:"0,keyasint"`
	// ResourceType uint8  `cbor:"1,keyasint,omitempty,omitzero"`
}
