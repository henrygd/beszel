// Package agent handles the agent's SSH server and system stats collection.
package agent

import (
	"beszel"
	"beszel/internal/entities/container"
	"beszel/internal/entities/system"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/sensors"

	psutilNet "github.com/shirou/gopsutil/v4/net"
)

type Agent struct {
	hostname                string                                   // Hostname of the system
	kernelVersion           string                                   // Kernel version of the system
	cpuModel                string                                   // CPU model of the system
	cores                   int                                      // Number of cores of the system
	threads                 int                                      // Number of threads of the system
	sem                     chan struct{}                            // Semaphore to limit concurrent access to docker api
	debug                   bool                                     // true if LOG_LEVEL is set to debug
	fsNames                 []string                                 // List of filesystem device names being monitored
	fsStats                 map[string]*system.FsStats               // Keeps track of disk stats for each filesystem
	netInterfaces           map[string]struct{}                      // Stores all valid network interfaces
	netIoStats              *system.NetIoStats                       // Keeps track of bandwidth usage
	prevContainerStatsMap   map[string]*container.PrevContainerStats // Keeps track of container stats
	prevContainerStatsMutex *sync.Mutex                              // Mutex to prevent concurrent access to prevContainerStatsMap
	dockerClient            *http.Client                             // HTTP client to query docker api
	apiContainerList        *[]container.ApiInfo                     // List of containers from docker host
	sensorsContext          context.Context                          // Sensors context to override sys location
}

func NewAgent() *Agent {
	return &Agent{
		sem:                     make(chan struct{}, 15),
		prevContainerStatsMap:   make(map[string]*container.PrevContainerStats),
		prevContainerStatsMutex: &sync.Mutex{},
		netIoStats:              &system.NetIoStats{},
		dockerClient:            newDockerClient(),
		sensorsContext:          context.Background(),
	}
}

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

func (a *Agent) getDockerStats() ([]container.Stats, error) {
	resp, err := a.dockerClient.Get("http://localhost/containers/json")
	if err != nil {
		a.closeIdleConnections(err)
		return nil, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&a.apiContainerList); err != nil {
		slog.Error("Error decoding containers", "err", err)
		return nil, err
	}

	containersLength := len(*a.apiContainerList)
	containerStats := make([]container.Stats, containersLength)

	// store valid ids to clean up old container ids from map
	validIds := make(map[string]struct{}, containersLength)

	var wg sync.WaitGroup

	for i, ctr := range *a.apiContainerList {
		ctr.IdShort = ctr.Id[:12]
		validIds[ctr.IdShort] = struct{}{}
		// check if container is less than 1 minute old (possible restart)
		// note: can't use Created field because it's not updated on restart
		if strings.Contains(ctr.Status, "second") {
			// if so, remove old container data
			a.deleteContainerStatsSync(ctr.IdShort)
		}
		wg.Add(1)
		a.acquireSemaphore()
		go func() {
			defer a.releaseSemaphore()
			defer wg.Done()
			cstats, err := a.getContainerStats(ctr)
			if err != nil {
				// close idle connections if error is a network timeout
				isTimeout := a.closeIdleConnections(err)
				// delete container from map if not a timeout
				if !isTimeout {
					a.deleteContainerStatsSync(ctr.IdShort)
				}
				// retry once
				cstats, err = a.getContainerStats(ctr)
				if err != nil {
					slog.Error("Error getting container stats", "err", err)
				}
			}
			containerStats[i] = cstats
		}()
	}

	wg.Wait()

	// remove old / invalid container stats
	for id := range a.prevContainerStatsMap {
		if _, exists := validIds[id]; !exists {
			delete(a.prevContainerStatsMap, id)
		}
	}

	return containerStats, nil
}

func (a *Agent) getContainerStats(ctr container.ApiInfo) (container.Stats, error) {
	curStats := container.Stats{Name: ctr.Names[0][1:]}

	resp, err := a.dockerClient.Get("http://localhost/containers/" + ctr.IdShort + "/stats?stream=0&one-shot=1")
	if err != nil {
		return curStats, err
	}
	defer resp.Body.Close()

	// docker host container stats response
	var res container.ApiStats
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return curStats, err
	}

	// check if container has valid data, otherwise may be in restart loop (#103)
	if res.MemoryStats.Usage == 0 {
		return curStats, fmt.Errorf("%s - no memory stats - see https://github.com/henrygd/beszel/issues/144", curStats.Name)
	}

	// memory (https://docs.docker.com/reference/cli/docker/container/stats/)
	memCache := res.MemoryStats.Stats["inactive_file"]
	if memCache == 0 {
		memCache = res.MemoryStats.Stats["cache"]
	}
	usedMemory := res.MemoryStats.Usage - memCache

	a.prevContainerStatsMutex.Lock()
	defer a.prevContainerStatsMutex.Unlock()

	// add empty values if they doesn't exist in map
	prevStats, initialized := a.prevContainerStatsMap[ctr.IdShort]
	if !initialized {
		prevStats = &container.PrevContainerStats{}
		a.prevContainerStatsMap[ctr.IdShort] = prevStats
	}

	// cpu
	cpuDelta := res.CPUStats.CPUUsage.TotalUsage - prevStats.Cpu[0]
	systemDelta := res.CPUStats.SystemUsage - prevStats.Cpu[1]
	cpuPct := float64(cpuDelta) / float64(systemDelta) * 100
	if cpuPct > 100 {
		return curStats, fmt.Errorf("%s cpu pct greater than 100: %+v", curStats.Name, cpuPct)
	}
	prevStats.Cpu = [2]uint64{res.CPUStats.CPUUsage.TotalUsage, res.CPUStats.SystemUsage}

	// network
	var total_sent, total_recv uint64
	for _, v := range res.Networks {
		total_sent += v.TxBytes
		total_recv += v.RxBytes
	}
	var sent_delta, recv_delta float64
	// prevent first run from sending all prev sent/recv bytes
	if initialized {
		secondsElapsed := time.Since(prevStats.Net.Time).Seconds()
		sent_delta = float64(total_sent-prevStats.Net.Sent) / secondsElapsed
		recv_delta = float64(total_recv-prevStats.Net.Recv) / secondsElapsed
	}
	prevStats.Net.Sent = total_sent
	prevStats.Net.Recv = total_recv
	prevStats.Net.Time = time.Now()

	curStats.Cpu = twoDecimals(cpuPct)
	curStats.Mem = bytesToMegabytes(float64(usedMemory))
	curStats.NetworkSent = bytesToMegabytes(sent_delta)
	curStats.NetworkRecv = bytesToMegabytes(recv_delta)

	return curStats, nil
}

func (a *Agent) gatherStats() system.CombinedData {
	systemInfo, systemStats := a.getSystemStats()
	systemData := system.CombinedData{
		Stats: systemStats,
		Info:  systemInfo,
	}
	// add docker stats
	if containerStats, err := a.getDockerStats(); err == nil {
		systemData.Containers = containerStats
	}
	// add extra filesystems
	systemData.Stats.ExtraFs = make(map[string]*system.FsStats)
	for name, stats := range a.fsStats {
		if !stats.Root && stats.DiskTotal > 0 {
			systemData.Stats.ExtraFs[name] = stats
		}
	}
	return systemData
}

func (a *Agent) Run(pubKey []byte, addr string) {
	// Create map for disk stats
	a.fsStats = make(map[string]*system.FsStats)

	// Set up slog with a log level determined by the LOG_LEVEL env var
	if logLevelStr, exists := os.LookupEnv("LOG_LEVEL"); exists {
		switch strings.ToLower(logLevelStr) {
		case "debug":
			a.debug = true
			slog.SetLogLoggerLevel(slog.LevelDebug)
		case "warn":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "error":
			slog.SetLogLoggerLevel(slog.LevelError)
		}
	}

	// Set sensors context (allows overriding sys location for sensors)
	if sysSensors, exists := os.LookupEnv("SYS_SENSORS"); exists {
		slog.Info("SYS_SENSORS", "path", sysSensors)
		a.sensorsContext = context.WithValue(a.sensorsContext,
			common.EnvKey, common.EnvMap{common.HostSysEnvKey: sysSensors},
		)
	}

	a.initializeHostInfo()
	a.initializeDiskInfo()
	a.initializeNetIoStats()

	a.startServer(pubKey, addr)
}

// Sets initial / non-changing values about the host
func (a *Agent) initializeHostInfo() {
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
