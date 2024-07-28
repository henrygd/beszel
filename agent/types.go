package main

import "time"

type SystemData struct {
	Stats      *SystemStats      `json:"stats"`
	Info       *SystemInfo       `json:"info"`
	Containers []*ContainerStats `json:"container"`
}

type SystemInfo struct {
	Cores    int    `json:"c"`
	Threads  int    `json:"t"`
	CpuModel string `json:"m"`
	// Os       string  `json:"o"`
	Uptime  uint64  `json:"u"`
	Cpu     float64 `json:"cpu"`
	MemPct  float64 `json:"mp"`
	DiskPct float64 `json:"dp"`
}

type SystemStats struct {
	Cpu          float64 `json:"cpu"`
	Mem          float64 `json:"m"`
	MemUsed      float64 `json:"mu"`
	MemPct       float64 `json:"mp"`
	MemBuffCache float64 `json:"mb"`
	Disk         float64 `json:"d"`
	DiskUsed     float64 `json:"du"`
	DiskPct      float64 `json:"dp"`
	DiskRead     float64 `json:"dr"`
	DiskWrite    float64 `json:"dw"`
	NetworkSent  float64 `json:"ns"`
	NetworkRecv  float64 `json:"nr"`
}

type ContainerStats struct {
	Name string  `json:"n"`
	Cpu  float64 `json:"c"`
	Mem  float64 `json:"m"`
	// MemPct float64 `json:"mp"`
}

type Container struct {
	Id      string
	IdShort string
	Names   []string
	Status  string
	// Image   string
	// ImageID string
	// Command string
	// Created int64
	// Ports      []Port
	// SizeRw     int64 `json:",omitempty"`
	// SizeRootFs int64 `json:",omitempty"`
	// Labels     map[string]string
	// State      string
	// HostConfig struct {
	// 	NetworkMode string            `json:",omitempty"`
	// 	Annotations map[string]string `json:",omitempty"`
	// }
	// NetworkSettings *SummaryNetworkSettings
	// Mounts          []MountPoint
}

type CStats struct {
	// Common stats
	Read    time.Time `json:"read"`
	PreRead time.Time `json:"preread"`

	// Linux specific stats, not populated on Windows.
	// PidsStats  PidsStats  `json:"pids_stats,omitempty"`
	// BlkioStats BlkioStats `json:"blkio_stats,omitempty"`

	// Windows specific stats, not populated on Linux.
	NumProcs uint32 `json:"num_procs"`
	// StorageStats StorageStats `json:"storage_stats,omitempty"`

	// Shared stats
	CPUStats    CPUStats    `json:"cpu_stats,omitempty"`
	PreCPUStats CPUStats    `json:"precpu_stats,omitempty"` // "Pre"="Previous"
	MemoryStats MemoryStats `json:"memory_stats,omitempty"`
}

type CPUStats struct {
	// CPU Usage. Linux and Windows.
	CPUUsage CPUUsage `json:"cpu_usage"`

	// System Usage. Linux only.
	SystemUsage uint64 `json:"system_cpu_usage,omitempty"`

	// Online CPUs. Linux only.
	OnlineCPUs uint32 `json:"online_cpus,omitempty"`

	// Throttling Data. Linux only.
	// ThrottlingData ThrottlingData `json:"throttling_data,omitempty"`
}

type CPUUsage struct {
	// Total CPU time consumed.
	// Units: nanoseconds (Linux)
	// Units: 100's of nanoseconds (Windows)
	TotalUsage uint64 `json:"total_usage"`

	// Total CPU time consumed per core (Linux). Not used on Windows.
	// Units: nanoseconds.
	PercpuUsage []uint64 `json:"percpu_usage,omitempty"`

	// Time spent by tasks of the cgroup in kernel mode (Linux).
	// Time spent by all container processes in kernel mode (Windows).
	// Units: nanoseconds (Linux).
	// Units: 100's of nanoseconds (Windows). Not populated for Hyper-V Containers.
	UsageInKernelmode uint64 `json:"usage_in_kernelmode"`

	// Time spent by tasks of the cgroup in user mode (Linux).
	// Time spent by all container processes in user mode (Windows).
	// Units: nanoseconds (Linux).
	// Units: 100's of nanoseconds (Windows). Not populated for Hyper-V Containers
	UsageInUsermode uint64 `json:"usage_in_usermode"`
}

type MemoryStats struct {

	// current res_counter usage for memory
	Usage uint64 `json:"usage,omitempty"`
	Cache uint64 `json:"cache,omitempty"`
	// maximum usage ever recorded.
	MaxUsage uint64 `json:"max_usage,omitempty"`
	// TODO(vishh): Export these as stronger types.
	// all the stats exported via memory.stat.
	Stats map[string]uint64 `json:"stats,omitempty"`
	// number of times memory usage hits limits.
	Failcnt uint64 `json:"failcnt,omitempty"`
	Limit   uint64 `json:"limit,omitempty"`

	// committed bytes
	Commit uint64 `json:"commitbytes,omitempty"`
	// peak committed bytes
	CommitPeak uint64 `json:"commitpeakbytes,omitempty"`
	// private working set
	PrivateWorkingSet uint64 `json:"privateworkingset,omitempty"`
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
