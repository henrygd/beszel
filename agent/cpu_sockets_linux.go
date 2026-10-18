//go:build linux

package agent

import (
	"context"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"golang.org/x/sys/unix"
)

// cpuSockets returns the number of physical CPU packages, or 0 if unknown.
// gopsutil's PhysicalID is not usable here: it is empty on ARM and holds the
// per-hart id on RISC-V.
func cpuSockets([]cpu.InfoStat) int {
	if !packageIdsReliable(runtime.GOARCH) {
		kernel, _ := host.KernelVersion()
		if !kernelAtLeast(kernel, 6, 0) {
			return 0
		}
	}
	var set unix.CPUSet
	if err := unix.SchedGetaffinity(0, &set); err != nil {
		return 0
	}
	return countCpuPackages(effectiveSysRoot(context.Background()), set.IsSet)
}

// packageIdsReliable reports whether physical_package_id is a real socket id on
// older kernels too. Before Linux 6.0, device-tree systems (most ARM boards) got
// the cluster number, so a single big.LITTLE chip looked like several packages.
func packageIdsReliable(arch string) bool {
	return arch == "amd64" || arch == "386"
}

// kernelAtLeast reports whether a version like "6.1.0-25-amd64" is >= major.minor.
func kernelAtLeast(version string, major, minor int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	maj, err1 := strconv.Atoi(parts[0])
	minorDigits, _, _ := strings.Cut(parts[1], "-")
	min, err2 := strconv.Atoi(minorDigits)
	if err1 != nil || err2 != nil {
		return false
	}
	return maj > major || (maj == major && min >= minor)
}

// countCpuPackages counts distinct physical_package_id values under sysRoot for
// the CPUs this process may run on, so a cpuset-limited container only counts
// the sockets it actually uses.
func countCpuPackages(sysRoot string, allowed func(cpu int) bool) int {
	paths, _ := filepath.Glob(filepath.Join(sysRoot, "devices/system/cpu/cpu[0-9]*/topology/physical_package_id"))
	packages := make(map[string]struct{})
	for _, path := range paths {
		n, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(filepath.Dir(filepath.Dir(path))), "cpu"))
		if err != nil || !allowed(n) {
			continue
		}
		if id, ok := utils.ReadStringFileOK(path); ok && id != "" {
			packages[id] = struct{}{}
		}
	}
	return len(packages)
}
