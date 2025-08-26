package system

// TODO: this is confusing, make common package with common/types common/helpers etc

import (
	"beszel/internal/entities/container"
	"time"
)

type Stats struct {
	Cpu            float64             `json:"cpu" cbor:"0,keyasint"`
	MaxCpu         float64             `json:"cpum,omitempty" cbor:"1,keyasint,omitempty"`
	Mem            float64             `json:"m" cbor:"2,keyasint"`
	MemUsed        float64             `json:"mu" cbor:"3,keyasint"`
	MemPct         float64             `json:"mp" cbor:"4,keyasint"`
	MemBuffCache   float64             `json:"mb" cbor:"5,keyasint"`
	MemZfsArc      float64             `json:"mz,omitempty" cbor:"6,keyasint,omitempty"` // ZFS ARC memory
	Swap           float64             `json:"s,omitempty" cbor:"7,keyasint,omitempty"`
	SwapUsed       float64             `json:"su,omitempty" cbor:"8,keyasint,omitempty"`
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
	LoadAvg1       float64             `json:"l1,omitempty" cbor:"23,keyasint,omitempty,omitzero"`
	LoadAvg5       float64             `json:"l5,omitempty" cbor:"24,keyasint,omitempty,omitzero"`
	LoadAvg15      float64             `json:"l15,omitempty" cbor:"25,keyasint,omitempty,omitzero"`
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

type DiskInfo struct {
	Name   string `json:"n"`
	Model  string `json:"m,omitempty"`
	Vendor string `json:"v,omitempty"`
}

type NetworkInfo struct {
	Name   string `json:"n"`
	Vendor string `json:"v,omitempty"`
	Model  string `json:"m,omitempty"`
	Speed  string `json:"s,omitempty"`
}

type MemoryInfo struct {
	Total string `json:"t,omitempty"`
}

type CpuInfo struct {
	Model    string `json:"m"`
	SpeedGHz string `json:"s"`
	Arch     string `json:"a"`
	Cores    int    `json:"c"`
	Threads  int    `json:"t"`
}

type OsInfo struct {
	Family  string `json:"f"`
	Version string `json:"v"`
	Kernel  string `json:"k"`
}

type NetworkLocationInfo struct {
	PublicIP string `json:"ip,omitempty"`
	ISP      string `json:"isp,omitempty"`
	ASN      string `json:"asn,omitempty"`
}

type Info struct {
	Hostname      string        `json:"h" cbor:"0,keyasint"`
	KernelVersion string        `json:"k,omitempty" cbor:"1,keyasint,omitempty"`
	Threads       int           `json:"t,omitempty" cbor:"2,keyasint,omitempty"`
	Uptime        uint64        `json:"u" cbor:"3,keyasint"`
	Cpu           float64       `json:"cpu" cbor:"4,keyasint"`
	MemPct        float64       `json:"mp" cbor:"5,keyasint"`
	DiskPct       float64       `json:"dp" cbor:"6,keyasint"`
	Bandwidth     float64       `json:"b" cbor:"7,keyasint"`
	AgentVersion  string        `json:"v" cbor:"8,keyasint"`
	Podman        bool          `json:"p,omitempty" cbor:"9,keyasint,omitempty"`
	GpuPct        float64       `json:"g,omitempty" cbor:"10,keyasint,omitempty"`
	DashboardTemp float64       `json:"dt,omitempty" cbor:"11,keyasint,omitempty"`
	Os            Os            `json:"os" cbor:"12,keyasint"`
	LoadAvg5      float64       `json:"l5,omitempty" cbor:"13,keyasint,omitempty,omitzero"`
	LoadAvg15     float64       `json:"l15,omitempty" cbor:"14,keyasint,omitempty,omitzero"`
	Disks         []DiskInfo            `json:"d" cbor:"15,omitempty"`
	Networks      []NetworkInfo         `json:"n,omitempty" cbor:"16,omitempty"`
	Memory        []MemoryInfo          `json:"m,omitempty" cbor:"17,omitempty"`
	Cpus          []CpuInfo             `json:"c,omitempty" cbor:"18,omitempty"`
	Oses          []OsInfo              `json:"o,omitempty" cbor:"19,omitempty"`
	NetworkLoc    []NetworkLocationInfo `json:"nl,omitempty" cbor:"20,keyasint,omitempty"`
}

// Final data structure to return to the hub
type CombinedData struct {
	Stats      Stats              `json:"stats" cbor:"0,keyasint"`
	Info       Info               `json:"info" cbor:"1,keyasint"`
	Containers []*container.Stats `json:"container" cbor:"2,keyasint"`
}
