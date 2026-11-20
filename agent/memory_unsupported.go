//go:build !linux

package agent

func canSubtractZfsArc() bool { return true }

func containerMemoryMetrics(uint64, bool) (memoryMetrics, bool) {
	return memoryMetrics{}, false
}
