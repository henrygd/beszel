package system

import "beszel/internal/entities/container"

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

type SystemData struct {
	Stats      *SystemStats                `json:"stats"`
	Info       *SystemInfo                 `json:"info"`
	Containers []*container.ContainerStats `json:"container"`
}
