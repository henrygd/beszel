package main

import (
	"golang.org/x/crypto/ssh"
)

type Server struct {
	Host   string
	Port   string
	Status string
	Client *ssh.Client
}

type SystemData struct {
	Stats      SystemStats      `json:"stats"`
	Info       SystemInfo       `json:"info"`
	Containers []ContainerStats `json:"container"`
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
	Name        string  `json:"n"`
	Cpu         float64 `json:"c"`
	Mem         float64 `json:"m"`
	NetworkSent float64 `json:"ns"`
	NetworkRecv float64 `json:"nr"`
}

type EmailData struct {
	to   string
	subj string
	body string
}
