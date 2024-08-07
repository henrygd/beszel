package system

type SystemStats struct {
	Cpu          float64 `json:"cpu"`
	Mem          float64 `json:"m"`
	MemUsed      float64 `json:"mu"`
	MemPct       float64 `json:"mp"`
	MemBuffCache float64 `json:"mb"`
	Swap         float64 `json:"s"`
	SwapUsed     float64 `json:"su"`
	Disk         float64 `json:"d"`
	DiskUsed     float64 `json:"du"`
	DiskPct      float64 `json:"dp"`
	DiskRead     float64 `json:"dr"`
	DiskWrite    float64 `json:"dw"`
	NetworkSent  float64 `json:"ns"`
	NetworkRecv  float64 `json:"nr"`
}
