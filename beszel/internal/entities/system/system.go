package system

// TODO: this is confusing, make common package with common/types common/helpers etc

import (
	"beszel/internal/entities/container"
	"time"
)

type Stats struct {
	Cpu            float64             `json:"cpu" cbor:"0,keyasint"`
	MaxCpu         float64             `json:"cpum,omitempty" cbor:"1,keyasint,omitempty"`
	CpuUser        float64             `json:"cpuu" cbor:"31,keyasint"`
	CpuSystem      float64             `json:"cpus" cbor:"32,keyasint"`
	CpuIowait      float64             `json:"cpui" cbor:"33,keyasint"`
	CpuSteal       float64             `json:"cpusl" cbor:"34,keyasint"`
	Mem            float64             `json:"m" cbor:"2,keyasint"`
	MemUsed        float64             `json:"mu" cbor:"3,keyasint"`
	MemPct         float64             `json:"mp" cbor:"4,keyasint"`
	MemBuffCache   float64             `json:"mb" cbor:"5,keyasint"`
	MemZfsArc      float64             `json:"mz,omitempty" cbor:"6,keyasint,omitempty"` // ZFS ARC memory
	Swap           float64             `json:"s,omitempty" cbor:"7,keyasint,omitempty"`
	SwapUsed       float64             `json:"su,omitempty" cbor:"8,keyasint,omitempty"`
	SwapTotal      float64             `json:"st,omitempty" cbor:"35,keyasint,omitempty"`
	SwapCached     float64             `json:"sc,omitempty" cbor:"36,keyasint,omitempty"`
	DiskTotal      float64             `json:"d" cbor:"9,keyasint"`
	DiskUsed       float64             `json:"du" cbor:"10,keyasint"`
	DiskPct        float64             `json:"dp" cbor:"11,keyasint"`
	DiskReadPs     float64             `json:"dr" cbor:"12,keyasint"`
	DiskWritePs    float64             `json:"dw" cbor:"13,keyasint"`
	MaxDiskReadPs  float64             `json:"drm,omitempty" cbor:"14,keyasint,omitempty"`
	MaxDiskWritePs float64             `json:"dwm,omitempty" cbor:"15,keyasint,omitempty"`
	NetworkSent    float64             `json:"ns" cbor:"16,keyasint"`
	NetworkRecv    float64             `json:"nr" cbor:"17,keyasint"`
	MaxNetworkSent float64             `json:"nsm,omitempty" cbor:"18,keyasint,omitempty"`
	MaxNetworkRecv float64             `json:"nrm,omitempty" cbor:"19,keyasint,omitempty"`
	Temperatures   map[string]float64  `json:"t,omitempty" cbor:"20,keyasint,omitempty"`
	ExtraFs        map[string]*FsStats `json:"efs,omitempty" cbor:"21,keyasint,omitempty"`
	GPUData        map[string]GPUData  `json:"g,omitempty" cbor:"22,keyasint,omitempty"`
	LoadAvg1       float64             `json:"l1,omitempty" cbor:"23,keyasint,omitempty"`
	LoadAvg5       float64             `json:"l5,omitempty" cbor:"24,keyasint,omitempty"`
	LoadAvg15      float64             `json:"l15,omitempty" cbor:"25,keyasint,omitempty"`
	Bandwidth      [2]uint64           `json:"b,omitzero" cbor:"26,keyasint,omitzero"`  // [sent bytes, recv bytes]
	MaxBandwidth   [2]uint64           `json:"bm,omitzero" cbor:"27,keyasint,omitzero"` // [sent bytes, recv bytes]
	// TODO: remove other load fields in future release in favor of load avg array
	LoadAvg [3]float64 `json:"la,omitempty" cbor:"28,keyasint"`
	Battery [2]uint8   `json:"bat,omitzero" cbor:"29,keyasint,omitzero"` // [percent, charge state, current]
	MaxMem  float64    `json:"mm,omitempty" cbor:"30,keyasint,omitempty"`
}

type GPUData struct {
	Name        string  `json:"n" cbor:"0,keyasint"`
	Temperature float64 `json:"-"`
	MemoryUsed  float64 `json:"mu,omitempty" cbor:"1,keyasint,omitempty"`
	MemoryTotal float64 `json:"mt,omitempty" cbor:"2,keyasint,omitempty"`
	Usage       float64 `json:"u" cbor:"3,keyasint"`
	Power       float64 `json:"p,omitempty" cbor:"4,keyasint,omitempty"`
	Count       float64 `json:"-"`
}

type FsStats struct {
	Time           time.Time `json:"-"`
	Root           bool      `json:"-"`
	Mountpoint     string    `json:"-"`
	DisplayName    string    `json:"n"`
	DiskTotal      float64   `json:"d" cbor:"0,keyasint"`
	DiskUsed       float64   `json:"du" cbor:"1,keyasint"`
	TotalRead      uint64    `json:"-"`
	TotalWrite     uint64    `json:"-"`
	DiskReadPs     float64   `json:"r" cbor:"2,keyasint"`
	DiskWritePs    float64   `json:"w" cbor:"3,keyasint"`
	MaxDiskReadPS  float64   `json:"rm,omitempty" cbor:"4,keyasint,omitempty"`
	MaxDiskWritePS float64   `json:"wm,omitempty" cbor:"5,keyasint,omitempty"`
}

type NetIoStats struct {
	BytesRecv uint64
	BytesSent uint64
	Time      time.Time
	Name      string
}

type Os = uint8

const (
	Linux Os = iota
	Darwin
	Windows
	Freebsd
)

type InfoFsStats struct {
	DisplayName string  `json:"n"`
	DiskTotal   float64 `json:"d"`
	DiskUsed    float64 `json:"du"`
}

type Info struct {
	Hostname       string                  `json:"h" cbor:"0,keyasint"`
	KernelVersion  string                  `json:"k,omitempty" cbor:"1,keyasint,omitempty"`
	Cores          int                     `json:"c" cbor:"2,keyasint"`
	Threads        int                     `json:"t,omitempty" cbor:"3,keyasint,omitempty"`
	CpuModel       string                  `json:"m" cbor:"4,keyasint"`
	Uptime         uint64                  `json:"u" cbor:"5,keyasint"`
	Cpu            float64                 `json:"cpu" cbor:"6,keyasint"`
	MemPct         float64                 `json:"mp" cbor:"7,keyasint"`
	DiskPct        float64                 `json:"dp" cbor:"8,keyasint"`
	Bandwidth      float64                 `json:"b" cbor:"9,keyasint"`
	AgentVersion   string                  `json:"v" cbor:"10,keyasint"`
	Podman         bool                    `json:"p,omitempty" cbor:"11,keyasint,omitempty"`
	GpuPct         float64                 `json:"g,omitempty" cbor:"12,keyasint,omitempty"`
	DashboardTemp  float64                 `json:"dt,omitempty" cbor:"13,keyasint,omitempty"`
	Os             Os                      `json:"os" cbor:"14,keyasint"`
	LoadAvg1       float64                 `json:"l1,omitempty" cbor:"15,keyasint,omitempty"`
	LoadAvg5       float64                 `json:"l5,omitempty" cbor:"16,keyasint,omitempty"`
	LoadAvg15      float64                 `json:"l15,omitempty" cbor:"17,keyasint,omitempty"`
	BandwidthBytes uint64                  `json:"bb" cbor:"18,keyasint"`
	// TODO: remove load fields in future release in favor of load avg array
	LoadAvg  [3]float64              `json:"la,omitempty" cbor:"19,keyasint"`
	ExtraFs  map[string]*InfoFsStats `json:"efs,omitempty" cbor:"20,keyasint,omitempty"`
}

// Final data structure to return to the hub
type CombinedData struct {
	Stats      Stats              `json:"stats" cbor:"0,keyasint"`
	Info       Info               `json:"info" cbor:"1,keyasint"`
	Containers []*container.Stats `json:"container" cbor:"2,keyasint"`
}
