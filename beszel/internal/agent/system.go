package agent

import (
	"beszel"
	"beszel/internal/agent/battery"
	"beszel/internal/entities/system"
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// Sets initial / non-changing values about the host system
func (a *Agent) initializeSystemInfo() {
	a.systemInfo.AgentVersion = beszel.Version
	a.systemInfo.Hostname, _ = os.Hostname()

	platform, _, version, _ := host.PlatformInformation()

	if platform == "darwin" {
		a.systemInfo.KernelVersion = version
		a.systemInfo.Os = system.Darwin
	} else if strings.Contains(platform, "indows") {
		a.systemInfo.KernelVersion = strings.Replace(platform, "Microsoft ", "", 1) + " " + version
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

// Returns current info, stats about the host system
func (a *Agent) getSystemStats() system.Stats {
	systemStats := system.Stats{}

	// battery
	if battery.HasReadableBattery() {
		systemStats.Battery[0], systemStats.Battery[1], _ = battery.GetBatteryStats()
	}

	// cpu percent
	cpuPct, err := cpu.Percent(0, false)
	if err != nil {
		slog.Error("Error getting cpu percent", "err", err)
	} else if len(cpuPct) > 0 {
		systemStats.Cpu = twoDecimals(cpuPct[0])
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
		// cache + buffers value for default mem calculation
		cacheBuff := v.Total - v.Free - v.Used
		// htop memory calculation overrides
		if a.memCalc == "htop" {
			// note: gopsutil automatically adds SReclaimable to v.Cached
			cacheBuff = v.Cached + v.Buffers - v.Shared
			v.Used = v.Total - (v.Free + cacheBuff)
			v.UsedPercent = float64(v.Used) / float64(v.Total) * 100.0
		}
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
	for _, stats := range a.fsStats {
		if d, err := disk.Usage(stats.Mountpoint); err == nil {
			stats.DiskTotal = bytesToGigabytes(d.Total)
			stats.DiskUsed = bytesToGigabytes(d.Used)
			if stats.Root {
				systemStats.DiskTotal = bytesToGigabytes(d.Total)
				systemStats.DiskUsed = bytesToGigabytes(d.Used)
				systemStats.DiskPct = twoDecimals(d.UsedPercent)
			}
		} else {
			// reset stats if error (likely unmounted)
			slog.Error("Error getting disk stats", "name", stats.Mountpoint, "err", err)
			stats.DiskTotal = 0
			stats.DiskUsed = 0
			stats.TotalRead = 0
			stats.TotalWrite = 0
		}
	}

	// disk i/o
	if ioCounters, err := disk.IOCounters(a.fsNames...); err == nil {
		for _, d := range ioCounters {
			stats := a.fsStats[d.Name]
			if stats == nil {
				continue
			}
			secondsElapsed := time.Since(stats.Time).Seconds()
			readPerSecond := bytesToMegabytes(float64(d.ReadBytes-stats.TotalRead) / secondsElapsed)
			writePerSecond := bytesToMegabytes(float64(d.WriteBytes-stats.TotalWrite) / secondsElapsed)
			// check for invalid values and reset stats if so
			if readPerSecond < 0 || writePerSecond < 0 || readPerSecond > 50_000 || writePerSecond > 50_000 {
				slog.Warn("Invalid disk I/O. Resetting.", "name", d.Name, "read", readPerSecond, "write", writePerSecond)
				a.initializeDiskIoStats(ioCounters)
				break
			}
			stats.Time = time.Now()
			stats.DiskReadPs = readPerSecond
			stats.DiskWritePs = writePerSecond
			stats.TotalRead = d.ReadBytes
			stats.TotalWrite = d.WriteBytes
			// if root filesystem, update system stats
			if stats.Root {
				systemStats.DiskReadPs = stats.DiskReadPs
				systemStats.DiskWritePs = stats.DiskWritePs
			}
		}
	}

	// network stats
	if len(a.netInterfaces) == 0 {
		// if no network interfaces, initialize again
		// this is a fix if agent started before network is online (#466)
		a.initializeNetIoStats()
	}
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		now := time.Now()

		// pre-allocate maps with known capacity
		interfaceCount := len(a.netInterfaces)
		if systemStats.NetworkInterfaces == nil || len(systemStats.NetworkInterfaces) != interfaceCount {
			systemStats.NetworkInterfaces = make(map[string]system.NetworkInterfaceStats, interfaceCount)
		}

		var totalSent, totalRecv float64

		// single pass through interfaces
		for _, v := range netIO {
			// skip if not in valid network interfaces list
			if _, exists := a.netInterfaces[v.Name]; !exists {
				continue
			}

			// get previous stats for this interface
			prevStats, exists := a.netIoStats[v.Name]
			var networkSentPs, networkRecvPs float64

			if exists {
				secondsElapsed := time.Since(prevStats.Time).Seconds()
				if secondsElapsed > 0 {
					// direct calculation to MB/s, avoiding intermediate bytes/sec
					networkSentPs = bytesToMegabytes(float64(v.BytesSent-prevStats.BytesSent) / secondsElapsed)
					networkRecvPs = bytesToMegabytes(float64(v.BytesRecv-prevStats.BytesRecv) / secondsElapsed)
				}
			}

			// accumulate totals
			totalSent += networkSentPs
			totalRecv += networkRecvPs

			// store per-interface stats
			systemStats.NetworkInterfaces[v.Name] = system.NetworkInterfaceStats{
				NetworkSent:    networkSentPs,
				NetworkRecv:    networkRecvPs,
				TotalBytesSent: v.BytesSent,
				TotalBytesRecv: v.BytesRecv,
			}

			// update previous stats (reuse existing struct if possible)
			if prevStats.Name == v.Name {
				prevStats.BytesRecv = v.BytesRecv
				prevStats.BytesSent = v.BytesSent
				prevStats.PacketsSent = v.PacketsSent
				prevStats.PacketsRecv = v.PacketsRecv
				prevStats.Time = now
				a.netIoStats[v.Name] = prevStats
			} else {
				a.netIoStats[v.Name] = system.NetIoStats{
					BytesRecv:   v.BytesRecv,
					BytesSent:   v.BytesSent,
					PacketsSent: v.PacketsSent,
					PacketsRecv: v.PacketsRecv,
					Time:        now,
					Name:        v.Name,
				}
			}
		}

		// add check for issue (#150) where sent is a massive number
		if totalSent > 10_000 || totalRecv > 10_000 {
			slog.Warn("Invalid net stats. Resetting.", "sent", totalSent, "recv", totalRecv)
			// reset network I/O stats
			a.initializeNetIoStats()
		} else {
			systemStats.NetworkSent = totalSent
			systemStats.NetworkRecv = totalRecv
		}
	}

	// connection counts
	a.updateConnectionCounts(&systemStats)

	// temperatures
	// TODO: maybe refactor to methods on systemStats
	a.updateTemperatures(&systemStats)

	// GPU data
	if a.gpuManager != nil {
		// reset high gpu percent
		a.systemInfo.GpuPct = 0
		// get current GPU data
		if gpuData := a.gpuManager.GetCurrentData(); len(gpuData) > 0 {
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
	a.systemInfo.Cpu = systemStats.Cpu
	a.systemInfo.LoadAvg = systemStats.LoadAvg
	// TODO: remove these in future release in favor of load avg array
	a.systemInfo.LoadAvg1 = systemStats.LoadAvg[0]
	a.systemInfo.LoadAvg5 = systemStats.LoadAvg[1]
	a.systemInfo.LoadAvg15 = systemStats.LoadAvg[2]
	a.systemInfo.MemPct = systemStats.MemPct
	a.systemInfo.DiskPct = systemStats.DiskPct
	a.systemInfo.Uptime, _ = host.Uptime()

	// Sum all per-interface network sent/recv and assign to systemInfo
	var totalSent, totalRecv float64
	for _, iface := range systemStats.NetworkInterfaces {
		totalSent += iface.NetworkSent
		totalRecv += iface.NetworkRecv
	}
	a.systemInfo.NetworkSent = twoDecimals(totalSent)
	a.systemInfo.NetworkRecv = twoDecimals(totalRecv)
	slog.Debug("sysinfo", "data", a.systemInfo)

	return systemStats
}

func (a *Agent) updateConnectionCounts(systemStats *system.Stats) {
	// Get IPv4 connections
	connectionsIPv4, err := psutilNet.Connections("inet")
	if err != nil {
		slog.Debug("Failed to get IPv4 connection stats", "err", err)
		return
	}

	// Get IPv6 connections
	connectionsIPv6, err := psutilNet.Connections("inet6")
	if err != nil {
		slog.Debug("Failed to get IPv6 connection stats", "err", err)
		// Continue with IPv4 only if IPv6 fails
	}

	// Initialize Nets map if needed
	if systemStats.Nets == nil {
		systemStats.Nets = make(map[string]float64)
	}

	// Count IPv4 connection states
	connStatsIPv4 := map[string]int{
		"established": 0,
		"listen":      0,
		"time_wait":   0,
		"close_wait":  0,
		"syn_recv":    0,
	}

	for _, conn := range connectionsIPv4 {
		// Only count TCP connections (Type 1 = SOCK_STREAM)
		if conn.Type == 1 {
			switch strings.ToUpper(conn.Status) {
			case "ESTABLISHED":
				connStatsIPv4["established"]++
			case "LISTEN":
				connStatsIPv4["listen"]++
			case "TIME_WAIT":
				connStatsIPv4["time_wait"]++
			case "CLOSE_WAIT":
				connStatsIPv4["close_wait"]++
			case "SYN_RECV":
				connStatsIPv4["syn_recv"]++
			}
		}
	}

	// Count IPv6 connection states
	connStatsIPv6 := map[string]int{
		"established": 0,
		"listen":      0,
		"time_wait":   0,
		"close_wait":  0,
		"syn_recv":    0,
	}

	for _, conn := range connectionsIPv6 {
		// Only count TCP connections (Type 1 = SOCK_STREAM)
		if conn.Type == 1 {
			switch strings.ToUpper(conn.Status) {
			case "ESTABLISHED":
				connStatsIPv6["established"]++
			case "LISTEN":
				connStatsIPv6["listen"]++
			case "TIME_WAIT":
				connStatsIPv6["time_wait"]++
			case "CLOSE_WAIT":
				connStatsIPv6["close_wait"]++
			case "SYN_RECV":
				connStatsIPv6["syn_recv"]++
			}
		}
	}

	// Add IPv4 connection counts to Nets
	systemStats.Nets["conn_established"] = float64(connStatsIPv4["established"])
	systemStats.Nets["conn_listen"] = float64(connStatsIPv4["listen"])
	systemStats.Nets["conn_timewait"] = float64(connStatsIPv4["time_wait"])
	systemStats.Nets["conn_closewait"] = float64(connStatsIPv4["close_wait"])
	systemStats.Nets["conn_synrecv"] = float64(connStatsIPv4["syn_recv"])

	// Add IPv6 connection counts to Nets
	systemStats.Nets["conn6_established"] = float64(connStatsIPv6["established"])
	systemStats.Nets["conn6_listen"] = float64(connStatsIPv6["listen"])
	systemStats.Nets["conn6_timewait"] = float64(connStatsIPv6["time_wait"])
	systemStats.Nets["conn6_closewait"] = float64(connStatsIPv6["close_wait"])
	systemStats.Nets["conn6_synrecv"] = float64(connStatsIPv6["syn_recv"])
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
