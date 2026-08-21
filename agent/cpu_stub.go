//go:build !linux && !darwin && !windows

package agent

func getCpuFrequencies() []float64 {
	return nil
}

func getCpuBaseClockMHz() float64 {
	return 0
}
