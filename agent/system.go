package agent

import (
	"bufio"
	"errors"
	"fmt"
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
		hostInfo, _ = a.dockerManager.GetHostInfo()
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
	if batteryPercent, batteryState, err := battery.GetBatteryStats(); err == nil {
		systemStats.Battery[0] = batteryPercent
		systemStats.Battery[1] = batteryState
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
		systemStats.LoadAvg[0] = avgstat.Load1
		systemStats.LoadAvg[1] = avgstat.Load5
		systemStats.LoadAvg[2] = avgstat.Load15
		slog.Debug("Load average", "5m", avgstat.Load5, "15m", avgstat.Load15)
	} else {
		slog.Error("Error getting load average", "err", err)
	}

	// memory
	if v, err := mem.VirtualMemory(); err == nil {
		// swap
		systemStats.Swap = utils.BytesToGigabytes(v.SwapTotal)
		systemStats.SwapUsed = utils.BytesToGigabytes(v.SwapTotal - v.SwapFree - v.SwapCached)
		// cache + buffers value for default mem calculation
		// note: gopsutil automatically adds SReclaimable to v.Cached
		cacheBuff := v.Cached + v.Buffers - v.Shared
		if cacheBuff <= 0 {
			cacheBuff = max(v.Total-v.Free-v.Used, 0)
		}
		// htop memory calculation overrides (likely outdated as of mid 2025)
		if a.memCalc == "htop" {
			// cacheBuff = v.Cached + v.Buffers - v.Shared
			v.Used = v.Total - (v.Free + cacheBuff)
			v.UsedPercent = float64(v.Used) / float64(v.Total) * 100.0
		}
		// if a.memCalc == "legacy" {
		// 	v.Used = v.Total - v.Free - v.Buffers - v.Cached
		// 	cacheBuff = v.Total - v.Free - v.Used
		// 	v.UsedPercent = float64(v.Used) / float64(v.Total) * 100.0
		// }
		// subtract ZFS ARC size from used memory and add as its own category
		if a.zfs {
			if arcSize, _ := zfs.ARCSize(); arcSize > 0 && arcSize < v.Used {
				v.Used = v.Used - arcSize
				v.UsedPercent = float64(v.Used) / float64(v.Total) * 100.0
				systemStats.MemZfsArc = utils.BytesToGigabytes(arcSize)
			}
		}
		systemStats.Mem = utils.BytesToGigabytes(v.Total)
		systemStats.MemBuffCache = utils.BytesToGigabytes(cacheBuff)
		systemStats.MemUsed = utils.BytesToGigabytes(v.Used)
		systemStats.MemPct = utils.TwoDecimals(v.UsedPercent)
	}

	// disk usage
	a.updateDiskUsage(&systemStats)

	// disk i/o (cache-aware per interval)
	a.updateDiskIo(cacheTimeMs, &systemStats)

	// network stats (per cache interval)
	a.updateNetworkStats(cacheTimeMs, &systemStats)

	// temperatures
	// TODO: maybe refactor to methods on systemStats
	a.updateTemperatures(&systemStats)

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
	a.systemInfo.Uptime, _ = host.Uptime()
	a.systemInfo.BandwidthBytes = systemStats.Bandwidth[0] + systemStats.Bandwidth[1]
	a.systemInfo.Threads = a.systemDetails.Threads

	return systemStats
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
