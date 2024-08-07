package container

type ContainerStats struct {
	Name        string  `json:"n"`
	Cpu         float64 `json:"c"`
	Mem         float64 `json:"m"`
	NetworkSent float64 `json:"ns"`
	NetworkRecv float64 `json:"nr"`
}
