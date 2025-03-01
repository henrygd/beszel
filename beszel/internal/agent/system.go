package agent

import (
	"beszel"
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
	"github.com/shirou/gopsutil/v4/mem"
	psutilNet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

// Sets initial / non-changing values about the host system
func (a *Agent) initializeSystemInfo() {
	a.systemInfo.AgentVersion = beszel.Version
	a.systemInfo.Hostname, _ = os.Hostname()
	a.systemInfo.KernelVersion, _ = host.KernelVersion()

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
	if _, err := getARCSize(); err == nil {
		a.zfs = true
	} else {
		slog.Debug("Not monitoring ZFS ARC", "err", err)
	}
}

// Returns current info, stats about the host system
func (a *Agent) getSystemStats() system.Stats {
	systemStats := system.Stats{}

	// cpu percent
	cpuPct, err := cpu.Percent(0, false)
	if err != nil {
		slog.Error("Error getting cpu percent", "err", err)
	} else if len(cpuPct) > 0 {
		systemStats.Cpu = twoDecimals(cpuPct[0])
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
		// maybe refactor this in the future to not cache interface names at all so we
		// don't miss an interface that's been added after agent started in any circumstance
		a.initializeNetIoStats()
	}
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		secondsElapsed := time.Since(a.netIoStats.Time).Seconds()
		a.netIoStats.Time = time.Now()
		bytesSent := uint64(0)
		bytesRecv := uint64(0)
		// sum all bytes sent and received
		for _, v := range netIO {
			// skip if not in valid network interfaces list
			if _, exists := a.netInterfaces[v.Name]; !exists {
				continue
			}
			bytesSent += v.BytesSent
			bytesRecv += v.BytesRecv
		}
		// add to systemStats
		sentPerSecond := float64(bytesSent-a.netIoStats.BytesSent) / secondsElapsed
		recvPerSecond := float64(bytesRecv-a.netIoStats.BytesRecv) / secondsElapsed
		networkSentPs := bytesToMegabytes(sentPerSecond)
		networkRecvPs := bytesToMegabytes(recvPerSecond)
		// add check for issue (#150) where sent is a massive number
		if networkSentPs > 10_000 || networkRecvPs > 10_000 {
			slog.Warn("Invalid net stats. Resetting.", "sent", networkSentPs, "recv", networkRecvPs)
			for _, v := range netIO {
				if _, exists := a.netInterfaces[v.Name]; !exists {
					continue
				}
				slog.Info(v.Name, "recv", v.BytesRecv, "sent", v.BytesSent)
			}
			// reset network I/O stats
			a.initializeNetIoStats()
		} else {
			systemStats.NetworkSent = networkSentPs
			systemStats.NetworkRecv = networkRecvPs
			// update netIoStats
			a.netIoStats.BytesSent = bytesSent
			a.netIoStats.BytesRecv = bytesRecv
		}
	}

	// temperatures (skip if sensors whitelist is set to empty string)
	err = a.updateTemperatures(&systemStats)
	if err != nil {
		slog.Error("Error getting temperatures", "err", err)
	}

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
			for _, gpu := range gpuData {
				if gpu.Temperature > 0 {
					systemStats.Temperatures[gpu.Name] = gpu.Temperature
				}
				// update high gpu percent for dashboard
				a.systemInfo.GpuPct = max(a.systemInfo.GpuPct, gpu.Usage)
			}
		}
	}

	// update base system info
	a.systemInfo.Cpu = systemStats.Cpu
	a.systemInfo.MemPct = systemStats.MemPct
	a.systemInfo.DiskPct = systemStats.DiskPct
	a.systemInfo.Uptime, _ = host.Uptime()
	a.systemInfo.Bandwidth = twoDecimals(systemStats.NetworkSent + systemStats.NetworkRecv)
	slog.Debug("sysinfo", "data", a.systemInfo)

	return systemStats
}

func (a *Agent) updateTemperatures(systemStats *system.Stats) error {
	// skip if sensors whitelist is set to empty string
	if a.sensorsWhitelist != nil && len(a.sensorsWhitelist) == 0 {
		slog.Debug("Skipping temperature collection")
		return nil
	}

	primarySensor, primarySensorIsDefined := GetEnv("PRIMARY_SENSOR")

	// reset high temp
	a.systemInfo.DashboardTemp = 0

	// get sensor data
	temps, err := sensors.TemperaturesWithContext(a.sensorsContext)
	if err != nil {
		return err
	}
	slog.Debug("Temperature", "sensors", temps)

	// return if no sensors
	if len(temps) == 0 {
		return nil
	}

	systemStats.Temperatures = make(map[string]float64, len(temps))
	for i, sensor := range temps {
		// skip if temperature is unreasonable
		if sensor.Temperature <= 0 || sensor.Temperature >= 200 {
			continue
		}
		sensorName := sensor.SensorKey
		if _, ok := systemStats.Temperatures[sensorName]; ok {
			// if key already exists, append int to key
			sensorName = sensorName + "_" + strconv.Itoa(i)
		}
		// skip if not in whitelist
		if a.sensorsWhitelist != nil {
			if _, nameInWhitelist := a.sensorsWhitelist[sensorName]; !nameInWhitelist {
				continue
			}
		}
		// set dashboard temperature
		if primarySensorIsDefined {
			if sensorName == primarySensor {
				a.systemInfo.DashboardTemp = sensor.Temperature
			}
		} else {
			a.systemInfo.DashboardTemp = max(a.systemInfo.DashboardTemp, sensor.Temperature)
		}
		systemStats.Temperatures[sensorName] = twoDecimals(sensor.Temperature)
	}
	return nil
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
