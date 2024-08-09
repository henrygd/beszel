package container

import "time"

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

type ContainerStats struct {
	Name        string  `json:"n"`
	Cpu         float64 `json:"c"`
	Mem         float64 `json:"m"`
	NetworkSent float64 `json:"ns"`
	NetworkRecv float64 `json:"nr"`
}

type PrevContainerStats struct {
	Cpu [2]uint64
	Net struct {
		Sent uint64
		Recv uint64
		Time time.Time
	}
}
