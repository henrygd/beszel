package systemd

import (
	"math"
	"runtime"
	"time"
)

// ServiceState represents the status of a systemd service
type ServiceState uint8

const (
	StatusActive ServiceState = iota
	StatusInactive
	StatusFailed
	StatusActivating
	StatusDeactivating
	StatusReloading
)

// ServiceSubState represents the sub status of a systemd service
type ServiceSubState uint8

const (
	SubStateDead ServiceSubState = iota
	SubStateRunning
	SubStateExited
	SubStateFailed
	SubStateUnknown
)

// ParseServiceStatus converts a string status to a ServiceStatus enum value
func ParseServiceStatus(status string) ServiceState {
	switch status {
	case "active":
		return StatusActive
	case "inactive":
		return StatusInactive
	case "failed":
		return StatusFailed
	case "activating":
		return StatusActivating
	case "deactivating":
		return StatusDeactivating
	case "reloading":
		return StatusReloading
	default:
		return StatusInactive
	}
}

// ParseServiceSubState converts a string sub status to a ServiceSubState enum value
func ParseServiceSubState(subState string) ServiceSubState {
	switch subState {
	case "dead":
		return SubStateDead
	case "running":
		return SubStateRunning
	case "exited":
		return SubStateExited
	case "failed":
		return SubStateFailed
	default:
		return SubStateUnknown
	}
}

// Service represents a single systemd service with its stats.
type Service struct {
	Name         string          `json:"n" cbor:"0,keyasint"`
	State        ServiceState    `json:"s" cbor:"1,keyasint"`
	Cpu          float64         `json:"c" cbor:"2,keyasint"`
	Mem          uint64          `json:"m" cbor:"3,keyasint"`
	MemPeak      uint64          `json:"mp" cbor:"4,keyasint"`
	Sub          ServiceSubState `json:"ss" cbor:"5,keyasint"`
	CpuPeak      float64         `json:"cp" cbor:"6,keyasint"`
	PrevCpuUsage uint64          `json:"-"`
	PrevReadTime time.Time       `json:"-"`
}

// UpdateCPUPercent calculates the CPU usage percentage for the service.
func (s *Service) UpdateCPUPercent(cpuUsage uint64) {
	now := time.Now()

	if s.PrevReadTime.IsZero() || cpuUsage < s.PrevCpuUsage {
		s.Cpu = 0
		s.PrevCpuUsage = cpuUsage
		s.PrevReadTime = now
		return
	}

	duration := now.Sub(s.PrevReadTime).Nanoseconds()
	if duration <= 0 {
		s.PrevCpuUsage = cpuUsage
		s.PrevReadTime = now
		return
	}

	coreCount := int64(runtime.NumCPU())
	duration *= coreCount

	usageDelta := cpuUsage - s.PrevCpuUsage
	cpuPercent := float64(usageDelta) / float64(duration)
	s.Cpu = twoDecimals(cpuPercent * 100)

	if s.Cpu > s.CpuPeak {
		s.CpuPeak = s.Cpu
	}

	s.PrevCpuUsage = cpuUsage
	s.PrevReadTime = now
}

func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}

// ServiceDependency represents a unit that the service depends on.
type ServiceDependency struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ActiveState string `json:"activeState,omitempty"`
	SubState    string `json:"subState,omitempty"`
}

// ServiceDetails contains extended information about a systemd service.
type ServiceDetails map[string]any
