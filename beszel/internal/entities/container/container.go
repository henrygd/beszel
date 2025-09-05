package container

import "time"

// Docker container info from /containers/json
type ApiInfo struct {
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

// Docker container resources from /containers/{id}/stats
type ApiStats struct {
	Read        time.Time `json:"read"`               // Time of stats generation
	NumProcs    uint32    `json:"num_procs,omitzero"` // Windows specific, not populated on Linux.
	Networks    map[string]NetworkStats
	CPUStats    CPUStats    `json:"cpu_stats"`
	MemoryStats MemoryStats `json:"memory_stats"`
}

func (s *ApiStats) CalculateCpuPercentLinux(prevCpuContainer uint64, prevCpuSystem uint64) float64 {
	cpuDelta := s.CPUStats.CPUUsage.TotalUsage - prevCpuContainer
	systemDelta := s.CPUStats.SystemUsage - prevCpuSystem

	// Avoid division by zero and handle first run case
	if systemDelta == 0 || prevCpuContainer == 0 {
		return 0.0
	}

	return float64(cpuDelta) / float64(systemDelta) * 100.0
}

// from: https://github.com/docker/cli/blob/master/cli/command/container/stats_helpers.go#L185
func (s *ApiStats) CalculateCpuPercentWindows(prevCpuUsage uint64, prevRead time.Time) float64 {
	// Max number of 100ns intervals between the previous time read and now
	possIntervals := uint64(s.Read.Sub(prevRead).Nanoseconds())
	possIntervals /= 100                // Convert to number of 100ns intervals
	possIntervals *= uint64(s.NumProcs) // Multiple by the number of processors

	// Intervals used
	intervalsUsed := s.CPUStats.CPUUsage.TotalUsage - prevCpuUsage

	// Percentage avoiding divide-by-zero
	if possIntervals > 0 {
		return float64(intervalsUsed) / float64(possIntervals) * 100.0
	}
	return 0.00
}

type CPUStats struct {
	// CPU Usage. Linux and Windows.
	CPUUsage CPUUsage `json:"cpu_usage"`
	// System Usage. Linux only.
	SystemUsage uint64 `json:"system_cpu_usage,omitempty"`
}

type CPUUsage struct {
	// Total CPU time consumed.
	// Units: nanoseconds (Linux)
	// Units: 100's of nanoseconds (Windows)
	TotalUsage uint64 `json:"total_usage"`
}

type MemoryStats struct {
	// current res_counter usage for memory
	Usage uint64 `json:"usage,omitempty"`
	// all the stats exported via memory.stat.
	Stats MemoryStatsStats `json:"stats"`
	// private working set (Windows only)
	PrivateWorkingSet uint64 `json:"privateworkingset,omitempty"`
}

type MemoryStatsStats struct {
	Cache        uint64 `json:"cache,omitempty"`
	InactiveFile uint64 `json:"inactive_file,omitempty"`
}

type NetworkStats struct {
	// Bytes received. Windows and Linux.
	RxBytes uint64 `json:"rx_bytes"`
	// Bytes sent. Windows and Linux.
	TxBytes uint64 `json:"tx_bytes"`
}

type prevNetStats struct {
	Sent uint64
	Recv uint64
}

// Docker container stats
type Stats struct {
	Name        string  `json:"n" cbor:"0,keyasint"`
	Cpu         float64 `json:"c" cbor:"1,keyasint"`
	Mem         float64 `json:"m" cbor:"2,keyasint"`
	NetworkSent float64 `json:"ns" cbor:"3,keyasint"`
	NetworkRecv float64 `json:"nr" cbor:"4,keyasint"`
	// PrevCpu     [2]uint64    `json:"-"`
	CpuSystem    uint64       `json:"-"`
	CpuContainer uint64       `json:"-"`
	PrevNet      prevNetStats `json:"-"`
	PrevReadTime time.Time    `json:"-"`
}
