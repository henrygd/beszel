//go:build !linux

package agent

// containerCpuMetrics is Linux-only (cgroup accounting). Other platforms keep
// the gopsutil /proc path.
func containerCpuMetrics(uint16) (CpuMetrics, bool) {
	return CpuMetrics{}, false
}
