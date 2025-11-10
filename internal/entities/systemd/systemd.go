package systemd

import (
	"runtime"
	"time"
)

// Service represents a single systemd service with its stats.
type Service struct {
	Name         string    `json:"n" cbor:"0,keyasint"`
	Status       string    `json:"s" cbor:"1,keyasint"`
	Cpu          float64   `json:"c" cbor:"2,keyasint"`
	Mem          float64   `json:"m" cbor:"3,keyasint"`
	PrevCpuUsage uint64    `json:"-"`
	PrevReadTime time.Time `json:"-"`
}

// CalculateCPUPercent calculates the CPU usage percentage for the service.
func (s *Service) CalculateCPUPercent(cpuUsage uint64) {
	if s.PrevReadTime.IsZero() {
		s.Cpu = 0
	} else {
		duration := time.Since(s.PrevReadTime).Nanoseconds()
		if duration > 0 {
			coreCount := int64(runtime.NumCPU())
			duration *= coreCount
			cpuPercent := float64(cpuUsage-s.PrevCpuUsage) / float64(duration)
			s.Cpu = cpuPercent * 100
		}
	}

	s.PrevCpuUsage = cpuUsage
	s.PrevReadTime = time.Now()
}
