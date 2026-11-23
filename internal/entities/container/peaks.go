package container

// Peaks preserves one-minute maxima through retention tiers without changing the wire protocol.
type Peaks struct {
	Cpu       float64   `json:"c"`
	Mem       float64   `json:"m"`
	Bandwidth [2]uint64 `json:"b"`
}
