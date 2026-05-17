//go:build !linux

package agent

func getCpuFrequencies() []float64 {
	return nil
}
