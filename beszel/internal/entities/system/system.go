package system

import (
	"beszel/internal/entities/container"
	"time"
)

type Stats struct {
	Cpu             float64             `json:"cpu"`
	PeakCpu         float64             `json:"pcpu,omitempty"`
	Mem             float64             `json:"m"`
	MemUsed         float64             `json:"mu"`
	MemPct          float64             `json:"mp"`
	MemBuffCache    float64             `json:"mb"`
	MemZfsArc       float64             `json:"mz,omitempty"` // ZFS ARC memory
	Swap            float64             `json:"s,omitempty"`
	SwapUsed        float64             `json:"su,omitempty"`
	DiskTotal       float64             `json:"d"`
	DiskUsed        float64             `json:"du"`
	DiskPct         float64             `json:"dp"`
	DiskReadPs      float64             `json:"dr"`
	DiskWritePs     float64             `json:"dw"`
	NetworkSent     float64             `json:"ns"`
	PeakNetworkSent float64             `json:"pns,omitempty"`
	NetworkRecv     float64             `json:"nr"`
	PeakNetworkRecv float64             `json:"pnr,omitempty"`
	Temperatures    map[string]float64  `json:"t,omitempty"`
	ExtraFs         map[string]*FsStats `json:"efs,omitempty"`
}

type FsStats struct {
	Time        time.Time `json:"-"`
	Root        bool      `json:"-"`
	Mountpoint  string    `json:"-"`
	DiskTotal   float64   `json:"d"`
	DiskUsed    float64   `json:"du"`
	TotalRead   uint64    `json:"-"`
	TotalWrite  uint64    `json:"-"`
	DiskWritePs float64   `json:"w"`
	DiskReadPs  float64   `json:"r"`
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
	AgentVersion  string  `json:"v"`
}

// Final data structure to return to the hub
type CombinedData struct {
	Stats      Stats              `json:"stats"`
	Info       Info               `json:"info"`
	Containers []*container.Stats `json:"container"`
}
