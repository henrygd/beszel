//go:build linux

package agent

import (
	"context"
	"path/filepath"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/shirou/gopsutil/v4/cpu"
)

// cpuSockets returns the number of physical CPU packages. gopsutil's PhysicalID
// is not usable here: it is empty on ARM and holds the per-hart id on RISC-V.
func cpuSockets([]cpu.InfoStat) int {
	return countCpuPackages(effectiveSysRoot(context.Background()))
}

// countCpuPackages counts distinct physical_package_id values under sysRoot.
func countCpuPackages(sysRoot string) int {
	paths, _ := filepath.Glob(filepath.Join(sysRoot, "devices/system/cpu/cpu[0-9]*/topology/physical_package_id"))
	packages := make(map[string]struct{})
	for _, path := range paths {
		if id, ok := utils.ReadStringFileOK(path); ok && id != "" {
			packages[id] = struct{}{}
		}
	}
	return len(packages)
}
