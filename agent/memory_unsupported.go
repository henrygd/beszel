//go:build !linux

package agent

func containerMemoryMetrics(uint64, bool) (memoryMetrics, bool) {
	return memoryMetrics{}, false
}
