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
	Cpu      float64 `json:"cpu"`
	Mem      float64 `json:"mem"`
	MemUsed  float64 `json:"memUsed"`
	MemPct   float64 `json:"memPct"`
	Disk     float64 `json:"disk"`
	DiskUsed float64 `json:"diskUsed"`
	DiskPct  float64 `json:"diskPct"`
}

type ContainerStats struct {
	Name   string  `json:"name"`
	Cpu    float64 `json:"cpu"`
	Mem    float64 `json:"mem"`
	MemPct float64 `json:"memPct"`
}
