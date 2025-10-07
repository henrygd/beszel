package agent

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent/battery"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

// prevDisk stores previous per-device disk counters for a given cache interval
type prevDisk struct {
	readBytes  uint64
	writeBytes uint64
	at         time.Time
}

// Sets initial / non-changing values about the host system
func (a *Agent) initializeSystemInfo() {
	a.systemInfo.AgentVersion = beszel.Version
	a.systemInfo.Hostname, _ = os.Hostname()

	platform, _, version, _ := host.PlatformInformation()

	if platform == "darwin" {
		a.systemInfo.KernelVersion = version
		a.systemInfo.Os = system.Darwin
	} else if strings.Contains(platform, "indows") {
		a.systemInfo.KernelVersion = fmt.Sprintf("%s %s", strings.Replace(platform, "Microsoft ", "", 1), version)
		a.systemInfo.Os = system.Windows
	} else if platform == "freebsd" {
		a.systemInfo.Os = system.Freebsd
		a.systemInfo.KernelVersion = version
	} else {
		a.systemInfo.Os = system.Linux
	}

	if a.systemInfo.KernelVersion == "" {
		a.systemInfo.KernelVersion, _ = host.KernelVersion()
	}

	// cpu model
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		a.systemInfo.CpuModel = info[0].ModelName
	}
	// cores / threads
	a.systemInfo.Cores, _ = cpu.Counts(false)
	if threads, err := cpu.Counts(true); err == nil {
		if threads > 0 && threads < a.systemInfo.Cores {
			// in lxc logical cores reflects container limits, so use that as cores if lower
			a.systemInfo.Cores = threads
		} else {
			a.systemInfo.Threads = threads
		}
	}

	// zfs
	if _, err := getARCSize(); err != nil {
		slog.Debug("Not monitoring ZFS ARC", "err", err)
	} else {
		a.zfs = true
	}
}

// getProcessStateCounts returns count of processes by state
func getProcessStateCounts() map[string]int {
	states := make(map[string]int)
	pids, err := process.Pids()
	if err != nil {
		slog.Debug("Error getting process PIDs", "err", err)
		return states
	}

	for _, pid := range pids {
		proc, err := process.NewProcess(pid)
		if err != nil {
			continue // Process might have disappeared
		}
		
		status, err := proc.Status()
		if err != nil {
			continue
		}
		
		// Status returns a slice, we want the first element
		if len(status) == 0 {
			continue
		}
		
		// Normalize status names - be more comprehensive with state detection
		statusLower := strings.ToLower(status[0])
		switch {
		case statusLower == "r" || strings.Contains(statusLower, "running"):
			states["running"]++
		case statusLower == "s" || strings.Contains(statusLower, "sleep") || strings.Contains(statusLower, "interruptible"):
			states["sleeping"]++
		case statusLower == "d" || strings.Contains(statusLower, "disk") || strings.Contains(statusLower, "uninterruptible"):
			states["disk_sleep"]++
		case statusLower == "z" || strings.Contains(statusLower, "zombie") || strings.Contains(statusLower, "defunct"):
			states["zombie"]++
		case statusLower == "t" || strings.Contains(statusLower, "stop") || strings.Contains(statusLower, "traced"):
			states["stopped"]++
		case statusLower == "i" || strings.Contains(statusLower, "idle"):
			states["idle"]++
		case statusLower == "w" || strings.Contains(statusLower, "wait"):
			states["sleeping"]++ // Waiting processes are essentially sleeping
		default:
			states["other"]++
		}
	}
	
	return states
}

// getInodeStats returns inode usage for filesystems
func getInodeStats(mountpoint string) (used, total uint64, err error) {
	var stat syscall.Statfs_t
	err = syscall.Statfs(mountpoint, &stat)
	if err != nil {
		return 0, 0, err
	}
	
	total = stat.Files
	used = total - stat.Ffree
	return used, total, nil
}

// Returns current info, stats about the host system
func (a *Agent) getSystemStats(cacheTimeMs uint16) system.Stats {
	var systemStats system.Stats
	// Initialize network interfaces map to ensure it's never nil
	systemStats.NetworkInterfaces = make(map[string][4]uint64)

	// battery
	if battery.HasReadableBattery() {
		systemStats.Battery[0], systemStats.Battery[1], _ = battery.GetBatteryStats()
	}

	// cpu percent
	cpuPercent, err := getCpuPercent(cacheTimeMs)
	if err == nil {
		systemStats.Cpu = twoDecimals(cpuPercent)
	} else {
		slog.Error("Error getting cpu percent", "err", err)
	}
	// detailed cpu times
	if cpuTimes, err := cpu.Times(false); err == nil && len(cpuTimes) > 0 {
		// Get the first CPU (total across all cores)
		currentCpu := cpuTimes[0]

		if len(a.prevCpuTimes) > 0 {
			prevCpu := a.prevCpuTimes[0]

			// Calculate deltas
			totalDelta := currentCpu.Total() - prevCpu.Total()
			if totalDelta > 0 {
				userDelta := currentCpu.User - prevCpu.User
				systemDelta := currentCpu.System - prevCpu.System
				iowaitDelta := currentCpu.Iowait - prevCpu.Iowait
				stealDelta := currentCpu.Steal - prevCpu.Steal

				// Calculate percentages
				systemStats.CpuUser = twoDecimals((userDelta / totalDelta) * 100)
				systemStats.CpuSystem = twoDecimals((systemDelta / totalDelta) * 100)
				systemStats.CpuIowait = twoDecimals((iowaitDelta / totalDelta) * 100)
				systemStats.CpuSteal = twoDecimals((stealDelta / totalDelta) * 100)
			}
		} else {
			// First run, initialize with zeros
			systemStats.CpuUser = 0
			systemStats.CpuSystem = 0
			systemStats.CpuIowait = 0
			systemStats.CpuSteal = 0
		}

		// Store current times for next iteration
		a.prevCpuTimes = cpuTimes
	} else {
		// If we can't get CPU times, set all detailed metrics to 0
		systemStats.CpuUser = 0
		systemStats.CpuSystem = 0
		systemStats.CpuIowait = 0
		systemStats.CpuSteal = 0
		if err != nil {
			slog.Debug("Error getting CPU times", "err", err)
		}
	}

	// per-core cpu times
	if perCoreTimes, err := cpu.Times(true); err == nil && len(perCoreTimes) > 0 {
		systemStats.CpuCores = make(map[string]system.CpuCoreStats)

		for i, currentCore := range perCoreTimes {
			coreId := currentCore.CPU
			coreStats := system.CpuCoreStats{}

			if len(a.prevPerCoreTimes) > i {
				prevCore := a.prevPerCoreTimes[i]

				// Calculate deltas
				totalDelta := currentCore.Total() - prevCore.Total()
				if totalDelta > 0 {
					userDelta := currentCore.User - prevCore.User
					systemDelta := currentCore.System - prevCore.System
					iowaitDelta := currentCore.Iowait - prevCore.Iowait
					stealDelta := currentCore.Steal - prevCore.Steal

					// Calculate percentages
					coreStats.CpuUser = twoDecimals((userDelta / totalDelta) * 100)
					coreStats.CpuSystem = twoDecimals((systemDelta / totalDelta) * 100)
					coreStats.CpuIowait = twoDecimals((iowaitDelta / totalDelta) * 100)
					coreStats.CpuSteal = twoDecimals((stealDelta / totalDelta) * 100)
				}
			}

			systemStats.CpuCores[coreId] = coreStats
		}

		// Store current per-core times for next iteration
		a.prevPerCoreTimes = perCoreTimes
	} else {
		if err != nil {
			slog.Debug("Error getting per-core CPU times", "err", err)
		}
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
		systemStats.Swap = bytesToGigabytes(v.SwapTotal)
		systemStats.SwapUsed = bytesToGigabytes(v.SwapTotal - v.SwapFree - v.SwapCached)
		systemStats.SwapTotal = bytesToGigabytes(v.SwapTotal)
		systemStats.SwapCached = bytesToGigabytes(v.SwapCached)
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
			if arcSize, _ := getARCSize(); arcSize > 0 && arcSize < v.Used {
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

	// update base system info
	a.systemInfo.ConnectionType = a.connectionManager.ConnectionType
	a.systemInfo.Cpu = systemStats.Cpu
	a.systemInfo.LoadAvg = systemStats.LoadAvg
	// TODO: remove these in future release in favor of load avg array
	a.systemInfo.LoadAvg1 = systemStats.LoadAvg[0]
	a.systemInfo.LoadAvg5 = systemStats.LoadAvg[1]
	a.systemInfo.LoadAvg15 = systemStats.LoadAvg[2]
	a.systemInfo.MemPct = systemStats.MemPct
	a.systemInfo.DiskPct = systemStats.DiskPct
	a.systemInfo.Uptime, _ = host.Uptime()
	// TODO: in future release, remove MB bandwidth values in favor of bytes
	a.systemInfo.Bandwidth = twoDecimals(systemStats.NetworkSent + systemStats.NetworkRecv)
	a.systemInfo.BandwidthBytes = systemStats.Bandwidth[0] + systemStats.Bandwidth[1]
	
	// process states
	systemStats.ProcessStates = getProcessStateCounts()

	slog.Debug("Network interfaces in stats", "ni", systemStats.NetworkInterfaces, "count", len(systemStats.NetworkInterfaces))

	// inode stats for filesystems
	for _, stats := range a.fsStats {
		if inodeUsed, inodeTotal, err := getInodeStats(stats.Mountpoint); err == nil {
			stats.InodeUsed = inodeUsed
			stats.InodeTotal = inodeTotal
			if inodeTotal > 0 {
				stats.InodePct = float64(inodeUsed) / float64(inodeTotal) * 100
			}
			
			// Update root filesystem inode stats
			if stats.Root {
				systemStats.InodeUsed = inodeUsed
				systemStats.InodeTotal = inodeTotal
				systemStats.InodePct = stats.InodePct
			}
		} else {
			slog.Debug("Error getting inode stats", "mountpoint", stats.Mountpoint, "err", err)
		}
	}
	
	slog.Debug("sysinfo", "data", a.systemInfo)

	return systemStats
}

// Returns the size of the ZFS ARC memory cache in bytes
func getARCSize() (uint64, error) {
	file, err := os.Open("/proc/spl/kstat/zfs/arcstats")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	// Scan the lines
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "size") {
			// Example line: size 4 15032385536
			fields := strings.Fields(line)
			if len(fields) < 3 {
				return 0, err
			}
			// Return the size as uint64
			return strconv.ParseUint(fields[2], 10, 64)
		}
	}

	return 0, fmt.Errorf("failed to parse size field")
}
