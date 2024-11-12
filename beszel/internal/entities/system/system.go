package system

import (
	"beszel/internal/entities/container"
	"time"
)

type Stats struct {
	Cpu            float64             `json:"cpu"`
	MaxCpu         float64             `json:"cpum,omitempty"`
	Mem            float64             `json:"m"`
	MemUsed        float64             `json:"mu"`
	MemPct         float64             `json:"mp"`
	MemBuffCache   float64             `json:"mb"`
	MemZfsArc      float64             `json:"mz,omitempty"` // ZFS ARC memory
	Swap           float64             `json:"s,omitempty"`
	SwapUsed       float64             `json:"su,omitempty"`
	DiskTotal      float64             `json:"d"`
	DiskUsed       float64             `json:"du"`
	DiskPct        float64             `json:"dp"`
	DiskReadPs     float64             `json:"dr"`
	DiskWritePs    float64             `json:"dw"`
	MaxDiskReadPs  float64             `json:"drm,omitempty"`
	MaxDiskWritePs float64             `json:"dwm,omitempty"`
	NetworkSent    float64             `json:"ns"`
	NetworkRecv    float64             `json:"nr"`
	MaxNetworkSent float64             `json:"nsm,omitempty"`
	MaxNetworkRecv float64             `json:"nrm,omitempty"`
	Temperatures   map[string]float64  `json:"t,omitempty"`
	ExtraFs        map[string]*FsStats `json:"efs,omitempty"`
	GPUData        map[string]GPUData  `json:"g,omitempty"`
}

type GPUData struct {
	Name        string  `json:"n"`
	Temperature float64 `json:"-"`
	MemoryUsed  float64 `json:"mu,omitempty"`
	MemoryTotal float64 `json:"mt,omitempty"`
	Usage       float64 `json:"u"`
	Power       float64 `json:"p,omitempty"`
	Count       float64 `json:"-"`
}

type FsStats struct {
	Time           time.Time `json:"-"`
	Root           bool      `json:"-"`
	Mountpoint     string    `json:"-"`
	DiskTotal      float64   `json:"d"`
	DiskUsed       float64   `json:"du"`
	TotalRead      uint64    `json:"-"`
	TotalWrite     uint64    `json:"-"`
	DiskReadPs     float64   `json:"r"`
	DiskWritePs    float64   `json:"w"`
	MaxDiskReadPS  float64   `json:"rm,omitempty"`
	MaxDiskWritePS float64   `json:"wm,omitempty"`
}

type NetIoStats struct {
	BytesRecv uint64
	BytesSent uint64
	Time      time.Time
	Name      string
}

type Info struct {
	Hostname      string  `json:"h"`
	KernelVersion string  `json:"k,omitempty"`
	Cores         int     `json:"c"`
	Threads       int     `json:"t,omitempty"`
	CpuModel      string  `json:"m"`
	Uptime        uint64  `json:"u"`
	Cpu           float64 `json:"cpu"`
	MemPct        float64 `json:"mp"`
	DiskPct       float64 `json:"dp"`
	Bandwidth     float64 `json:"b"`
	AgentVersion  string  `json:"v"`
	Podman        bool    `json:"p,omitempty"`
}

// Final data structure to return to the hub
type CombinedData struct {
	Stats      Stats              `json:"stats"`
	Info       Info               `json:"info"`
	Containers []*container.Stats `json:"container"`
}
