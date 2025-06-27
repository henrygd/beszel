package system

// TODO: this is confusing, make common package with common/types common/helpers etc

import (
	"beszel/internal/entities/container"
	"time"
)

type Stats struct {
	Cpu            float64             `json:"cpu"`
	MaxCpu         float64             `json:"cpum,omitempty"`
	CpuUser        float64             `json:"cu,omitempty"` // CPU user time
	CpuSystem      float64             `json:"cs,omitempty"` // CPU system time
	CpuIowait      float64             `json:"ci,omitempty"` // CPU IOWAIT time
	CpuSteal       float64             `json:"cst,omitempty"` // CPU steal time
	Mem            float64             `json:"m"`
	MemUsed        float64             `json:"mu"`
	MemPct         float64             `json:"mp"`
	MemBuffCache   float64             `json:"mb"`
	MemZfsArc      float64             `json:"mz,omitempty"` // ZFS ARC memory
	Swap           float64             `json:"s,omitempty"`
	SwapUsed       float64             `json:"su,omitempty"`
	SwapFree       float64             `json:"sf,omitempty"` // Swap free space
	SwapCached     float64             `json:"sc,omitempty"` // Swap cached
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

type Os uint8

const (
	Linux Os = iota
	Darwin
	Windows
	Freebsd
)

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
	GpuPct        float64 `json:"g,omitempty"`
	DashboardTemp float64 `json:"dt,omitempty"`
	Os            Os      `json:"os"`
	OsName        string  `json:"on,omitempty"`    // OS name (e.g., "Ubuntu", "CentOS", "Windows 11")
	OsVersion     string  `json:"ov,omitempty"`    // OS version (e.g., "22.04", "10.0.19045")
	OsArch        string  `json:"oa,omitempty"`    // OS architecture (e.g., "x86_64", "arm64")
}

// Final data structure to return to the hub
type CombinedData struct {
	Stats      Stats              `json:"stats"`
	Info       Info               `json:"info"`
	Containers []*container.Stats `json:"container"`
}
