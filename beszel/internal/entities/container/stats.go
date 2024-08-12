package container

import "time"

// Docker container resources info from /containers/id/stats
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

// Stats to return to the hub
type ContainerStats struct {
	Name        string  `json:"n"`
	Cpu         float64 `json:"c"`
	Mem         float64 `json:"m"`
	NetworkSent float64 `json:"ns"`
	NetworkRecv float64 `json:"nr"`
}

// Keeps track of container stats from previous run
type PrevContainerStats struct {
	Cpu [2]uint64
	Net struct {
		Sent uint64
		Recv uint64
		Time time.Time
	}
}
