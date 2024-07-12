package main

import (
	"golang.org/x/crypto/ssh"
)

type Server struct {
	Ip     string
	Port   string
	Client *ssh.Client
}

type SystemData struct {
	System     SystemStats      `json:"stats"`
	Containers []ContainerStats `json:"container"`
}

type SystemStats struct {
	Cpu      float64 `json:"c"`
	Mem      float64 `json:"m"`
	MemUsed  float64 `json:"mu"`
	MemPct   float64 `json:"mp"`
	MemBuf   float64 `json:"mb"`
	Disk     float64 `json:"d"`
	DiskUsed float64 `json:"du"`
	DiskPct  float64 `json:"dp"`
}

type ContainerStats struct {
	Name string  `json:"n"`
	Cpu  float64 `json:"c"`
	Mem  float64 `json:"m"`
	// MemPct float64 `json:"mp"`
}
