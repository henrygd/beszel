package system

// TODO: this is confusing, make common package with common/types common/helpers etc

import (
	"beszel/internal/entities/container"
	"time"
)

type Stats struct {
	Cpu            float64              `json:"cpu"`
	MaxCpu         float64              `json:"cpum,omitempty"`
	Mem            float64              `json:"m"`
	MemUsed        float64              `json:"mu"`
	MemPct         float64              `json:"mp"`
	MemBuffCache   float64              `json:"mb"`
	MemZfsArc      float64              `json:"mz,omitempty"` // ZFS ARC memory
	Swap           float64              `json:"s,omitempty"`
	SwapUsed       float64              `json:"su,omitempty"`
	DiskTotal      float64              `json:"d"`
	DiskUsed       float64              `json:"du"`
	DiskPct        float64              `json:"dp"`
	DiskReadPs     float64              `json:"dr"`
	DiskWritePs    float64              `json:"dw"`
	MaxDiskReadPs  float64              `json:"drm,omitempty"`
	MaxDiskWritePs float64              `json:"dwm,omitempty"`
	NetworkSent    float64              `json:"ns"`
	NetworkRecv    float64              `json:"nr"`
	MaxNetworkSent float64              `json:"nsm,omitempty"`
	MaxNetworkRecv float64              `json:"nrm,omitempty"`
	Temperatures   map[string]float64   `json:"t,omitempty"`
	ExtraFs        map[string]*FsStats  `json:"efs,omitempty"`
	GPUData        map[string]GPUData   `json:"g,omitempty"`
	SmartData      map[string]SmartData `json:"sm,omitempty"`
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

type SmartData struct {
	ModelFamily     string            `json:"smf,omitempty"`
	ModelName       string            `json:"smn,omitempty"`
	SerialNumber    string            `json:"ssn,omitempty"`
	FirmwareVersion string            `json:"sfv,omitempty"`
	Capacity        uint64            `json:"sc,omitempty"`
	SmartStatus     string            `json:"ss,omitempty"`
	DiskName        string            `json:"sdn,omitempty"` // something like /dev/sda
	DiskType        string            `json:"sdt,omitempty"`
	Temperature	    int               `json:"st,omitempty"`
	Attributes      []*SmartAttribute `json:"sa,omitempty"`
}

type SmartAttribute struct {
	Id         int    `json:"id,omitempty"`
	Name       string `json:"n"`
	Value      int    `json:"v"`
	Worst      int    `json:"w,omitempty"`
	Threshold  int    `json:"t,omitempty"`
	RawValue   int    `json:"rv,omitempty"`
	RawString  string `json:"rs,omitempty"`
	Flags      string `json:"f,omitempty"`
	WhenFailed string `json:"wf,omitempty"`
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
	GpuPct        float64 `json:"g,omitempty"`
	DashboardTemp float64 `json:"dt,omitempty"`
}

// Final data structure to return to the hub
type CombinedData struct {
	Stats      Stats              `json:"stats"`
	Info       Info               `json:"info"`
	Containers []*container.Stats `json:"container"`
}
