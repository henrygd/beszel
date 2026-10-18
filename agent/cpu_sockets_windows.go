//go:build windows

package agent

import "github.com/shirou/gopsutil/v4/cpu"

// cpuSockets returns the number of physical CPU packages. On Windows gopsutil
// returns one entry per package, all with the same SMBIOS PhysicalID.
func cpuSockets(info []cpu.InfoStat) int {
	return len(info)
}
