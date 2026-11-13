package system

// ChartMax holds independent one-minute peaks for metrics without legacy max fields.
// The hub computes these during retention aggregation. No agent protocol change is needed.
// Capacities, states and cumulative counters in nested records are context, not peaks.
type ChartMax struct {
	Temperatures      map[string]float64   `json:"t,omitzero"`
	Fans              map[string]uint16    `json:"f,omitzero"`
	LoadAvg           [3]float64           `json:"la,omitzero"`
	CpuBreakdown      []float64            `json:"cpub,omitzero"`
	CpuCoresUsage     Uint8Slice           `json:"cpus,omitzero"`
	MemBytes          [4]uint64            `json:"mem,omitzero"`
	SwapBytes         [2]uint64            `json:"swap,omitzero"`
	DiskUsed          float64              `json:"du,omitzero"`
	Battery           Battery              `json:"bat,omitzero"`
	Batteries         map[string]uint8     `json:"bats,omitzero"`
	WiFi              map[string]int8      `json:"wf,omitzero"`
	NetworkInterfaces map[string][4]uint64 `json:"ni,omitzero"`
	ExtraFs           map[string]*FsStats  `json:"efs,omitzero"`
	GPUData           map[string]GPUData   `json:"g,omitzero"`
	ZfsPools          map[string]*ZfsPool  `json:"z,omitzero"`
}
