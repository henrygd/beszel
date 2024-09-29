package agent

import (
	"beszel"
	"beszel/internal/entities/system"
	"log/slog"
	"os"
	"strconv"
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
	a.kernelVersion, _ = host.KernelVersion()
	a.hostname, _ = os.Hostname()

	// add cpu stats
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		a.cpuModel = info[0].ModelName
	}
	a.cores, _ = cpu.Counts(false)
	if threads, err := cpu.Counts(true); err == nil {
		if threads > 0 && threads < a.cores {
			// in lxc logical cores reflects container limits, so use that as cores if lower
			a.cores = threads
		} else {
			a.threads = threads
		}
	}
}

// Returns current info, stats about the host system
func (a *Agent) getSystemStats() (system.Info, system.Stats) {
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
		systemStats.Mem = bytesToGigabytes(v.Total)
		systemStats.MemUsed = bytesToGigabytes(v.Used)
		systemStats.MemBuffCache = bytesToGigabytes(v.Total - v.Free - v.Used)
		systemStats.MemPct = twoDecimals(v.UsedPercent)
		systemStats.Swap = bytesToGigabytes(v.SwapTotal)
		systemStats.SwapUsed = bytesToGigabytes(v.SwapTotal - v.SwapFree)
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
			readPerSecond := float64(d.ReadBytes-stats.TotalRead) / secondsElapsed
			writePerSecond := float64(d.WriteBytes-stats.TotalWrite) / secondsElapsed
			stats.Time = time.Now()
			stats.DiskReadPs = bytesToMegabytes(readPerSecond)
			stats.DiskWritePs = bytesToMegabytes(writePerSecond)
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
			slog.Warn("Invalid network stats. Resetting.", "sent", networkSentPs, "recv", networkRecvPs)
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

	// temperatures
	temps, err := sensors.TemperaturesWithContext(a.sensorsContext)
	if err != nil && a.debug {
		err.(*sensors.Warnings).Verbose = true
		slog.Debug("Sensor error", "errs", err)
	}
	if len(temps) > 0 {
		slog.Debug("Temperatures", "data", temps)
		systemStats.Temperatures = make(map[string]float64, len(temps))
		for i, sensor := range temps {
			// skip if temperature is 0
			if sensor.Temperature == 0 {
				continue
			}
			if _, ok := systemStats.Temperatures[sensor.SensorKey]; ok {
				// if key already exists, append int to key
				systemStats.Temperatures[sensor.SensorKey+"_"+strconv.Itoa(i)] = twoDecimals(sensor.Temperature)
			} else {
				systemStats.Temperatures[sensor.SensorKey] = twoDecimals(sensor.Temperature)
			}
		}
		// remove sensors from systemStats if whitelist exists and sensor is not in whitelist
		// (do this here instead of in initial loop so we have correct keys if int was appended)
		if a.sensorsWhitelist != nil {
			for key := range systemStats.Temperatures {
				if _, nameInWhitelist := a.sensorsWhitelist[key]; !nameInWhitelist {
					delete(systemStats.Temperatures, key)
				}
			}
		}
	}

	systemInfo := system.Info{
		Cpu:           systemStats.Cpu,
		MemPct:        systemStats.MemPct,
		DiskPct:       systemStats.DiskPct,
		AgentVersion:  beszel.Version,
		Hostname:      a.hostname,
		KernelVersion: a.kernelVersion,
		CpuModel:      a.cpuModel,
		Cores:         a.cores,
		Threads:       a.threads,
	}

	systemInfo.Uptime, _ = host.Uptime()

	return systemInfo, systemStats
}
