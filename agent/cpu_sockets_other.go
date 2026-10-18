//go:build !linux && !windows

package agent

import "github.com/shirou/gopsutil/v4/cpu"

// cpuSockets is not detected on this platform.
func cpuSockets([]cpu.InfoStat) int {
	return 0
}
