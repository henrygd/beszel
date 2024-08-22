package system

import (
	"beszel/internal/entities/container"
	"time"
)

type Stats struct {
	Cpu          float64            `json:"cpu"`
	Mem          float64            `json:"m"`
	MemUsed      float64            `json:"mu"`
	MemPct       float64            `json:"mp"`
	MemBuffCache float64            `json:"mb"`
	Swap         float64            `json:"s"`
	SwapUsed     float64            `json:"su"`
	Disk         float64            `json:"d"`
	DiskUsed     float64            `json:"du"`
	DiskPct      float64            `json:"dp"`
	DiskRead     float64            `json:"dr"`
	DiskWrite    float64            `json:"dw"`
	NetworkSent  float64            `json:"ns"`
	NetworkRecv  float64            `json:"nr"`
	Temperatures map[string]float64 `json:"t,omitempty"`
}

type DiskIoStats struct {
	Read       uint64
	Write      uint64
	Time       time.Time
	Filesystem string
}

type NetIoStats struct {
	BytesRecv uint64
	BytesSent uint64
	Time      time.Time
	Name      string
}

type Info struct {
	Cores    int    `json:"c"`
	Threads  int    `json:"t"`
	CpuModel string `json:"m"`
	// Os       string  `json:"o"`
	Uptime       uint64  `json:"u"`
	Cpu          float64 `json:"cpu"`
	MemPct       float64 `json:"mp"`
	DiskPct      float64 `json:"dp"`
	AgentVersion string  `json:"v"`
}

// Final data structure to return to the hub
type CombinedData struct {
	Stats      *Stats             `json:"stats"`
	Info       *Info              `json:"info"`
	Containers []*container.Stats `json:"container"`
}
