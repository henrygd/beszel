//go:build !linux

package agent

// containerCpuMetrics is Linux-only (cgroup accounting). Other platforms keep
// the gopsutil /proc path.
func containerCpuMetrics(uint16, bool) (CpuMetrics, bool) {
	return CpuMetrics{}, false
}

func (a *Agent) initializeCpu() {}

func (a *Agent) warnIfRootCgroup() {}
