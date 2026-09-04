package agent

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent/battery"
	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/agent/zfs"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// Sets initial / non-changing values about the host system
func (a *Agent) refreshSystemDetails() {
	a.systemInfo.AgentVersion = beszel.Version

	// get host info from Docker if available
	var hostInfo container.HostInfo

	if a.dockerManager != nil {
		a.systemDetails.Podman = a.dockerManager.IsPodman()
		// Docker's host info describes the machine its daemon runs on. On macOS and
		// Windows that is a Linux VM, so its CPU and memory totals are not this host's.
		if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
			hostInfo, _ = a.dockerManager.GetHostInfo()
		}
	}

	a.systemDetails.Hostname, _ = os.Hostname()
	if arch, err := host.KernelArch(); err == nil {
		a.systemDetails.Arch = arch
	} else {
		a.systemDetails.Arch = runtime.GOARCH
	}

	platform, _, version, _ := host.PlatformInformation()

	if platform == "darwin" {
		a.systemDetails.Os = system.Darwin
		a.systemDetails.OsName = fmt.Sprintf("macOS %s", version)
	} else if strings.Contains(platform, "indows") {
		a.systemDetails.Os = system.Windows
		a.systemDetails.OsName = strings.Replace(platform, "Microsoft ", "", 1)
		a.systemDetails.Kernel = version
	} else if platform == "freebsd" {
		a.systemDetails.Os = system.Freebsd
		a.systemDetails.Kernel, _ = host.KernelVersion()
		if prettyName, err := getOsPrettyName(); err == nil {
			a.systemDetails.OsName = prettyName
		} else {
			a.systemDetails.OsName = "FreeBSD"
		}
	} else {
		a.systemDetails.Os = system.Linux
		a.systemDetails.OsName = hostInfo.OperatingSystem
		if a.systemDetails.OsName == "" {
			if prettyName, err := getOsPrettyName(); err == nil {
				a.systemDetails.OsName = prettyName
			} else {
				a.systemDetails.OsName = platform
			}
		}
		a.systemDetails.Kernel = hostInfo.KernelVersion
		if a.systemDetails.Kernel == "" {
			a.systemDetails.Kernel, _ = host.KernelVersion()
		}
	}

	// cpu model
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		a.systemDetails.CpuModel = info[0].ModelName
	}
	// gopsutil doesn't parse the "cpu model" field from /proc/cpuinfo, which
	// is the only source of the CPU model name on MIPS. Fall back to reading
	// it directly when ModelName is empty.
	if a.systemDetails.CpuModel == "" {
		a.systemDetails.CpuModel = getCpuModelFromCpuinfo()
	}
	// cores / threads
	cores, _ := cpu.Counts(false)
	threads := hostInfo.NCPU
	if threads == 0 {
		threads, _ = cpu.Counts(true)
	}
	// in lxc, logical cores reflects container limits, so use that as cores if lower
	if threads > 0 && threads < cores {
		cores = threads
	}
	a.systemDetails.Cores = cores
	a.systemDetails.Threads = threads

	// total memory
	a.systemDetails.MemoryTotal = hostInfo.MemTotal
	if a.systemDetails.MemoryTotal == 0 {
		if v, err := mem.VirtualMemory(); err == nil {
			a.systemDetails.MemoryTotal = v.Total
		}
	}

	// zfs
	if _, err := zfs.ARCSize(); err != nil {
		slog.Debug("Not monitoring ZFS ARC", "err", err)
	} else {
		a.zfs = true
	}
}

// attachSystemDetails returns details only for fresh default-interval responses.
func (a *Agent) attachSystemDetails(data *system.CombinedData, cacheTimeMs uint16, includeRequested bool) *system.CombinedData {
	if cacheTimeMs != defaultDataCacheTimeMs || (!includeRequested && !a.detailsDirty) {
		return data
	}

	// copy data to avoid adding details to the original cached struct
	response := *data
	response.Details = &a.systemDetails
	a.detailsDirty = false
	return &response
}

// updateSystemDetails applies a mutation to the static details payload and marks
// it for inclusion on the next fresh default-interval response.
func (a *Agent) updateSystemDetails(updateFunc func(details *system.Details)) {
	updateFunc(&a.systemDetails)
	a.detailsDirty = true
}

// Returns current info, stats about the host system
func (a *Agent) getSystemStats(cacheTimeMs uint16) system.Stats {
	var systemStats system.Stats

	// battery
	if batteries, err := battery.GetBatteryStats(); err == nil {
		systemStats.Batteries = make(map[string]uint8, len(batteries))
		for _, device := range batteries {
			systemStats.Batteries[device.Name] = device.Percent
		}
		if primary, ok := battery.Primary(batteries); ok {
			systemStats.Battery = [2]uint8{primary.Percent, primary.State}
		}
	}

	// cpu metrics
	cpuMetrics, err := getCpuMetrics(cacheTimeMs)
	if err == nil {
		systemStats.Cpu = utils.TwoDecimals(cpuMetrics.Total)
		systemStats.CpuBreakdown = []float64{
			utils.TwoDecimals(cpuMetrics.User),
			utils.TwoDecimals(cpuMetrics.System),
			utils.TwoDecimals(cpuMetrics.Iowait),
			utils.TwoDecimals(cpuMetrics.Steal),
			utils.TwoDecimals(cpuMetrics.Idle),
		}
	} else {
		slog.Error("Error getting cpu metrics", "err", err)
	}

	// per-core cpu usage
	if perCoreUsage, err := getPerCoreCpuUsage(cacheTimeMs); err == nil {
		systemStats.CpuCoresUsage = perCoreUsage
	}

	// load average
	if avgstat, err := load.Avg(); err == nil {
		systemStats.LoadAvg[0] = utils.TwoDecimals(avgstat.Load1)
		systemStats.LoadAvg[1] = utils.TwoDecimals(avgstat.Load5)
		systemStats.LoadAvg[2] = utils.TwoDecimals(avgstat.Load15)
		slog.Debug("Load average", "5m", avgstat.Load5, "15m", avgstat.Load15)
	} else {
		slog.Error("Error getting load average", "err", err)
	}

	// memory
	if v, err := mem.VirtualMemory(); err == nil {
		used, cacheBuff, swapUsed := calculateHostMemoryUsage(v, a.memCalc == "htop")
		// swap
		systemStats.Swap = utils.BytesToGigabytes(v.SwapTotal)
		systemStats.SwapUsed = utils.BytesToGigabytes(swapUsed)
		v.Used = used
		// if a.memCalc == "legacy" {
		// 	v.Used = v.Total - v.Free - v.Buffers - v.Cached
		// 	cacheBuff = v.Total - v.Free - v.Used
		// 	v.UsedPercent = float64(v.Used) / float64(v.Total) * 100.0
		// }
		// subtract ZFS ARC size from used memory and add as its own category
		if a.zfs {
			if arcSize, _ := zfs.ARCSize(); arcSize > 0 && arcSize < v.Used {
				v.Used = v.Used - arcSize
				systemStats.MemZfsArc = utils.BytesToGigabytes(arcSize)
			}
		}
		if v.Total > 0 {
			v.UsedPercent = float64(v.Used) / float64(v.Total) * 100.0
		} else {
			v.UsedPercent = 0
		}
		systemStats.Mem = utils.BytesToGigabytes(v.Total)
		systemStats.MemBuffCache = utils.BytesToGigabytes(cacheBuff)
		systemStats.MemUsed = utils.BytesToGigabytes(v.Used)
		systemStats.MemPct = utils.TwoDecimals(v.UsedPercent)
		systemStats.MemSlab = utils.BytesToGigabytes(v.Slab)
	}

	// swap I/O rates, OOM kills, and memory pressure (Linux only)
	a.updateMemExtras(cacheTimeMs, &systemStats)

	// disk usage
	a.updateDiskUsage(&systemStats)

	// disk i/o (cache-aware per interval)
	a.updateDiskIo(cacheTimeMs, &systemStats)

	// zfs pool stats
	a.zfsManager.Update(&systemStats)

	// network stats (per cache interval)
	a.updateNetworkStats(cacheTimeMs, &systemStats)

	// temperatures
	// TODO: maybe refactor to methods on systemStats
	a.updateTemperatures(&systemStats)

	// fan speeds (Linux-only; sysfs hwmon)
	a.updateFans(&systemStats)

	// GPU data
	if a.gpuManager != nil {
		// reset high gpu percent
		a.systemInfo.GpuPct = 0
		// get current GPU data
		if gpuData := a.gpuManager.GetCurrentData(cacheTimeMs); len(gpuData) > 0 {
			systemStats.GPUData = gpuData

			// add temperatures
			if systemStats.Temperatures == nil {
				systemStats.Temperatures = make(map[string]float64, len(gpuData))
			}
			highestTemp := 0.0
			for _, gpu := range gpuData {
				if gpu.Temperature > 0 {
					systemStats.Temperatures[gpu.Name] = gpu.Temperature
					if a.sensorConfig.primarySensor == gpu.Name {
						a.systemInfo.DashboardTemp = gpu.Temperature
					}
					if gpu.Temperature > highestTemp {
						highestTemp = gpu.Temperature
					}
				}
				// update high gpu percent for dashboard
				a.systemInfo.GpuPct = max(a.systemInfo.GpuPct, gpu.Usage)
			}
			// use highest temp for dashboard temp if dashboard temp is unset
			if a.systemInfo.DashboardTemp == 0 {
				a.systemInfo.DashboardTemp = highestTemp
			}
		}
	}

	// update system info
	a.systemInfo.ConnectionType = a.connectionManager.ConnectionType
	a.systemInfo.Cpu = systemStats.Cpu
	a.systemInfo.LoadAvg = systemStats.LoadAvg
	a.systemInfo.MemPct = systemStats.MemPct
	a.systemInfo.DiskPct = systemStats.DiskPct
	a.systemInfo.Battery = systemStats.Battery
	a.systemInfo.Uptime, _ = getUptime()
	a.systemInfo.BandwidthBytes = systemStats.Bandwidth[0] + systemStats.Bandwidth[1]
	a.systemInfo.Threads = a.systemDetails.Threads

	return systemStats
}

// cpuModelFallbackKeys are the field names to look for in /proc/cpuinfo when
// gopsutil fails to return a ModelName. The "cpu model" key is used on MIPS
// (e.g. "MIPS 1004Kc V2.15"), while "system type" provides SoC information
// on various embedded architectures.
var cpuModelFallbackKeys = []string{"cpu model", "system type"}

// getCpuModelFromCpuinfo reads /proc/cpuinfo and returns a CPU model string.
// This is a fallback for architectures where gopsutil's cpu.Info() does not
// populate ModelName, most notably MIPS.
func getCpuModelFromCpuinfo() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer file.Close()
	return parseCpuModel(file)
}

// parseCpuModel scans r (expected to be /proc/cpuinfo content) and returns
// a combined CPU model string. It collects values from all matching keys
// and joins them with " / " when multiple are found.
func parseCpuModel(r io.Reader) string {
	lines := readLines(r)
	var parts []string
	for _, key := range cpuModelFallbackKeys {
		for _, line := range lines {
			after, found := strings.CutPrefix(line, key)
			if !found {
				continue
			}
			after = strings.TrimSpace(after)
			if len(after) < 2 || after[0] != ':' {
				continue
			}
			if value := strings.TrimSpace(after[1:]); value != "" {
				parts = append(parts, value)
				break
			}
		}
	}
	return strings.Join(parts, " / ")
}

// readLines reads all lines from r into a slice.
func readLines(r io.Reader) []string {
	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// calculateHostMemoryUsage derives counters defensively because /proc/meminfo may
// change while gopsutil reads it. Invalid unsigned subtractions saturate at zero.
func calculateHostMemoryUsage(v *mem.VirtualMemoryStat, htop bool) (used, cacheBuff, swapUsed uint64) {
	used = v.Used
	if used > v.Total {
		used = saturatingSub(v.Total, v.Available)
	}

	// gopsutil automatically adds SReclaimable to Cached.
	cacheBuff = min(v.Cached, v.Total)
	cacheBuff += min(v.Buffers, v.Total-cacheBuff)
	cacheBuff = saturatingSub(cacheBuff, min(v.Shared, v.Total))
	if v.Cached == 0 && v.Buffers == 0 {
		cacheBuff = saturatingSub(v.Total, v.Free, used)
	}
	if htop {
		used = saturatingSub(v.Total, v.Free, cacheBuff)
	}
	// Cached swap pages still occupy swap slots and are included in `free`'s used value.
	return used, cacheBuff, saturatingSub(v.SwapTotal, v.SwapFree)
}

// saturatingSub subtracts each value, returning zero on underflow.
func saturatingSub(value uint64, subtrahends ...uint64) uint64 {
	for _, subtrahend := range subtrahends {
		if subtrahend > value {
			return 0
		}
		value -= subtrahend
	}
	return value
}

// getOsPrettyName attempts to get the pretty OS name from /etc/os-release on Linux systems
func getOsPrettyName() (string, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			value := after
			value = strings.Trim(value, `"`)
			return value, nil
		}
	}

	return "", errors.New("pretty name not found")
}
