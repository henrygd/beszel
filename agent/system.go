package agent

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent/battery"
	"github.com/henrygd/beszel/agent/zfs"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// prevDisk stores previous per-device disk counters for a given cache interval
type prevDisk struct {
	readBytes  uint64
	writeBytes uint64
	at         time.Time
}

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
		systemStats.Cpu = twoDecimals(cpuMetrics.Total)
		systemStats.CpuBreakdown = []float64{
			twoDecimals(cpuMetrics.User),
			twoDecimals(cpuMetrics.System),
			twoDecimals(cpuMetrics.Iowait),
			twoDecimals(cpuMetrics.Steal),
			twoDecimals(cpuMetrics.Idle),
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
		// Validate memory values - in some container environments (e.g., Oracle Cloud with cgroup2),
		// gopsutil may return invalid values when memory.max is set to "max" (unlimited).
		if v.Used > v.Total || v.UsedPercent > 100 {
			slog.Debug("Invalid memory stats from gopsutil, falling back to /proc/meminfo",
				"used", v.Used, "total", v.Total, "usedPercent", v.UsedPercent)
			if memInfo, err := parseMemInfo(); err == nil {
				v.Total = memInfo.Total
				v.Free = memInfo.Free
				v.Available = memInfo.Available
				v.Buffers = memInfo.Buffers
				v.Cached = memInfo.Cached
				v.Shared = memInfo.Shared
				v.SwapTotal = memInfo.SwapTotal
				v.SwapFree = memInfo.SwapFree
				v.SwapCached = memInfo.SwapCached

				// Calculate used memory with underflow protection
				freeAndCache := v.Free + v.Buffers + v.Cached
				if freeAndCache < v.Total {
					v.Used = v.Total - freeAndCache
				} else {
					v.Used = 0
				}
				if v.Total > 0 {
					v.UsedPercent = float64(v.Used) / float64(v.Total) * 100.0
				}
			}
		}

		// swap
		systemStats.Swap = bytesToGigabytes(v.SwapTotal)
		systemStats.SwapUsed = bytesToGigabytes(v.SwapTotal - v.SwapFree - v.SwapCached)
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
				systemStats.MemZfsArc = bytesToGigabytes(arcSize)
			}
		}
		systemStats.Mem = bytesToGigabytes(v.Total)
		systemStats.MemBuffCache = bytesToGigabytes(cacheBuff)
		systemStats.MemUsed = bytesToGigabytes(v.Used)
		systemStats.MemPct = twoDecimals(v.UsedPercent)
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

// memInfo holds memory information parsed from /proc/meminfo
type memInfo struct {
	Total      uint64
	Free       uint64
	Available  uint64
	Buffers    uint64
	Cached     uint64
	Shared     uint64
	SwapTotal  uint64
	SwapFree   uint64
	SwapCached uint64
}

// parseMemInfo reads /proc/meminfo directly and returns memory statistics.
func parseMemInfo() (*memInfo, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info := &memInfo{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Values in /proc/meminfo are in kB, convert to bytes
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		valueBytes := value * 1024

		switch fields[0] {
		case "MemTotal:":
			info.Total = valueBytes
		case "MemFree:":
			info.Free = valueBytes
		case "MemAvailable:":
			info.Available = valueBytes
		case "Buffers:":
			info.Buffers = valueBytes
		case "Cached:":
			info.Cached = valueBytes
		case "Shmem:":
			info.Shared = valueBytes
		case "SwapTotal:":
			info.SwapTotal = valueBytes
		case "SwapFree:":
			info.SwapFree = valueBytes
		case "SwapCached:":
			info.SwapCached = valueBytes
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if info.Total == 0 {
		return nil, fmt.Errorf("failed to parse MemTotal from /proc/meminfo")
	}

	return info, nil
}
