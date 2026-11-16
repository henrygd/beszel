// Package vm contains the compact wire and storage representation for virtual
// machines reported by the agent.
package vm

// State mirrors the stable virDomainState values exposed by libvirt.
type State uint8

const (
	StateNoState State = iota
	StateRunning
	StateBlocked
	StatePaused
	StateShutdown
	StateShutoff
	StateCrashed
	StatePMSuspended
)

// Stats is one virtual machine sample. Short keys keep the per-minute history
// compact. CPU is normalized against total host CPU capacity so VM series can
// be compared directly with the host CPU chart. GuestCPU is normalized against
// the VM's assigned vCPUs.
type Stats struct {
	Name           string    `json:"n" cbor:"0,keyasint"`
	UUID           string    `json:"id" cbor:"1,keyasint"`
	State          State     `json:"s" cbor:"2,keyasint"`
	CPU            float64   `json:"c" cbor:"3,keyasint"`
	GuestCPU       float64   `json:"cg,omitzero" cbor:"4,keyasint,omitempty"`
	Memory         float64   `json:"m,omitzero" cbor:"5,keyasint,omitempty"`  // MiB used inside the guest
	MemoryTotal    float64   `json:"mt,omitzero" cbor:"6,keyasint,omitempty"` // MiB visible to the guest
	MemoryFresh    bool      `json:"mf,omitzero" cbor:"7,keyasint,omitempty"`
	Bandwidth      [2]uint64 `json:"b,omitzero" cbor:"8,keyasint,omitempty"`   // bytes/s [sent, received], guest point of view
	DiskIO         [2]uint64 `json:"d,omitzero" cbor:"9,keyasint,omitempty"`   // bytes/s [read, written]
	DiskCapacity   uint64    `json:"dc,omitzero" cbor:"10,keyasint,omitempty"` // bytes
	DiskAllocation uint64    `json:"da,omitzero" cbor:"11,keyasint,omitempty"` // bytes
	VCPUs          uint16    `json:"v,omitzero" cbor:"12,keyasint,omitempty"`
	Autostart      bool      `json:"a,omitzero" cbor:"13,keyasint,omitempty"`
}
