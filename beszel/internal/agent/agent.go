// Package agent handles the agent's SSH server and system stats collection.
package agent

import (
	"beszel"
	"beszel/internal/entities/container"
	"beszel/internal/entities/system"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

	sshServer "github.com/gliderlabs/ssh"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

type Agent struct {
	addr                string
	pubKey              []byte
	sem                 chan struct{}
	containerStatsMap   map[string]*container.PrevContainerStats
	containerStatsMutex *sync.Mutex
	fsNames             []string
	fsStats             map[string]*system.FsStats
	netInterfaces       map[string]struct{}
	netIoStats          *system.NetIoStats
	dockerClient        *http.Client
	sensorsContext      context.Context
}

func NewAgent(pubKey []byte, addr string) *Agent {
	return &Agent{
		addr:                addr,
		pubKey:              pubKey,
		sem:                 make(chan struct{}, 15),
		containerStatsMap:   make(map[string]*container.PrevContainerStats),
		containerStatsMutex: &sync.Mutex{},
		netIoStats:          &system.NetIoStats{},
		dockerClient:        newDockerClient(),
		sensorsContext:      context.Background(),
	}
}

func (a *Agent) acquireSemaphore() {
	a.sem <- struct{}{}
}

func (a *Agent) releaseSemaphore() {
	<-a.sem
}

func (a *Agent) getSystemStats() (system.Info, system.Stats) {
	systemStats := system.Stats{}

	// cpu percent
	cpuPct, err := cpu.Percent(0, false)
	if err != nil {
		log.Println("Error getting cpu percent:", err)
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
		// log.Println("Reading filesystem:", fs.Mountpoint)
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
			log.Printf("Error reading %s: %+v\n", stats.Mountpoint, err)
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
			// log.Printf("%+v: %+v recv, %+v sent\n", v.Name, v.BytesRecv, v.BytesSent)
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
			log.Printf("Warning: network sent/recv is %.2f/%.2f MB/s. Resetting stats.\n", networkSentPs, networkRecvPs)
			for _, v := range netIO {
				if _, exists := a.netInterfaces[v.Name]; !exists {
					continue
				}
				log.Printf("%+s: %v recv, %v sent\n", v.Name, v.BytesRecv, v.BytesSent)
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
	if temps, err := sensors.TemperaturesWithContext(a.sensorsContext); err == nil {
		systemStats.Temperatures = make(map[string]float64)
		// log.Printf("Temperatures: %+v\n", temps)
		for i, temp := range temps {
			if _, ok := systemStats.Temperatures[temp.SensorKey]; ok {
				// if key already exists, append int to key
				systemStats.Temperatures[temp.SensorKey+"_"+strconv.Itoa(i)] = twoDecimals(temp.Temperature)
			} else {
				systemStats.Temperatures[temp.SensorKey] = twoDecimals(temp.Temperature)
			}
		}
		// log.Printf("Temperature map: %+v\n", systemStats.Temperatures)
	}

	systemInfo := system.Info{
		Cpu:          systemStats.Cpu,
		MemPct:       systemStats.MemPct,
		DiskPct:      systemStats.DiskPct,
		AgentVersion: beszel.Version,
	}

	// add host info
	if info, err := host.Info(); err == nil {
		systemInfo.Uptime = info.Uptime
		systemInfo.Hostname = info.Hostname
		systemInfo.KernelVersion = info.KernelVersion
	}

	// add cpu stats
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		systemInfo.CpuModel = info[0].ModelName
	}
	if cores, err := cpu.Counts(false); err == nil {
		systemInfo.Cores = cores
	}
	if threads, err := cpu.Counts(true); err == nil {
		if threads > 0 && threads < systemInfo.Cores {
			// in lxc logical cores reflects container limits, so use that as cores if lower
			systemInfo.Cores = threads
		} else {
			systemInfo.Threads = threads
		}
	}

	return systemInfo, systemStats
}

func (a *Agent) getDockerStats() ([]container.Stats, error) {
	resp, err := a.dockerClient.Get("http://localhost/containers/json")
	if err != nil {
		a.closeIdleConnections(err)
		return nil, err
	}
	defer resp.Body.Close()

	var containers []container.ApiInfo
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		log.Printf("Error decoding containers: %+v\n", err)
		return nil, err
	}

	containerStats := make([]container.Stats, 0, len(containers))
	containerStatsMutex := sync.Mutex{}

	// store valid ids to clean up old container ids from map
	validIds := make(map[string]struct{}, len(containers))

	var wg sync.WaitGroup

	for _, ctr := range containers {
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
					log.Printf("Error getting container stats: %+v\n", err)
					return
				}
			}
			containerStatsMutex.Lock()
			defer containerStatsMutex.Unlock()
			containerStats = append(containerStats, cstats)
		}()
	}

	wg.Wait()

	for id := range a.containerStatsMap {
		if _, exists := validIds[id]; !exists {
			// log.Printf("Removing container cpu map entry: %+v\n", id)
			delete(a.containerStatsMap, id)
		}
	}

	return containerStats, nil
}

func (a *Agent) getContainerStats(ctr container.ApiInfo) (container.Stats, error) {
	cStats := container.Stats{}

	resp, err := a.dockerClient.Get("http://localhost/containers/" + ctr.IdShort + "/stats?stream=0&one-shot=1")
	if err != nil {
		return cStats, err
	}
	defer resp.Body.Close()

	// decode the json data from the response body
	var statsJson container.ApiStats
	if err := json.NewDecoder(resp.Body).Decode(&statsJson); err != nil {
		return cStats, err
	}

	name := ctr.Names[0][1:]

	// check if container has valid data, otherwise may be in restart loop (#103)
	if statsJson.MemoryStats.Usage == 0 {
		return cStats, fmt.Errorf("%s - invalid data", name)
	}

	// memory (https://docs.docker.com/reference/cli/docker/container/stats/)
	memCache := statsJson.MemoryStats.Stats["inactive_file"]
	if memCache == 0 {
		memCache = statsJson.MemoryStats.Stats["cache"]
	}
	usedMemory := statsJson.MemoryStats.Usage - memCache

	a.containerStatsMutex.Lock()
	defer a.containerStatsMutex.Unlock()

	// add empty values if they doesn't exist in map
	stats, initialized := a.containerStatsMap[ctr.IdShort]
	if !initialized {
		stats = &container.PrevContainerStats{}
		a.containerStatsMap[ctr.IdShort] = stats
	}

	// cpu
	cpuDelta := statsJson.CPUStats.CPUUsage.TotalUsage - stats.Cpu[0]
	systemDelta := statsJson.CPUStats.SystemUsage - stats.Cpu[1]
	cpuPct := float64(cpuDelta) / float64(systemDelta) * 100
	if cpuPct > 100 {
		return cStats, fmt.Errorf("%s cpu pct greater than 100: %+v", name, cpuPct)
	}
	stats.Cpu = [2]uint64{statsJson.CPUStats.CPUUsage.TotalUsage, statsJson.CPUStats.SystemUsage}

	// network
	var total_sent, total_recv uint64
	for _, v := range statsJson.Networks {
		total_sent += v.TxBytes
		total_recv += v.RxBytes
	}
	var sent_delta, recv_delta float64
	// prevent first run from sending all prev sent/recv bytes
	if initialized {
		secondsElapsed := time.Since(stats.Net.Time).Seconds()
		sent_delta = float64(total_sent-stats.Net.Sent) / secondsElapsed
		recv_delta = float64(total_recv-stats.Net.Recv) / secondsElapsed
		// log.Printf("sent delta: %+v, recv delta: %+v\n", sent_delta, recv_delta)
	}
	stats.Net.Sent = total_sent
	stats.Net.Recv = total_recv
	stats.Net.Time = time.Now()

	// cStats := a.containerStatsPool.Get().(*container.Stats)
	cStats.Name = name
	cStats.Cpu = twoDecimals(cpuPct)
	cStats.Mem = bytesToMegabytes(float64(usedMemory))
	cStats.NetworkSent = bytesToMegabytes(sent_delta)
	cStats.NetworkRecv = bytesToMegabytes(recv_delta)

	return cStats, nil
}

// delete container stats from map using mutex
func (a *Agent) deleteContainerStatsSync(id string) {
	a.containerStatsMutex.Lock()
	defer a.containerStatsMutex.Unlock()
	delete(a.containerStatsMap, id)
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
	// log.Printf("%+v\n", systemData)
	return systemData
}

func (a *Agent) startServer() {
	sshServer.Handle(a.handleSession)

	log.Printf("Starting SSH server on %s", a.addr)
	if err := sshServer.ListenAndServe(a.addr, nil, sshServer.NoPty(),
		sshServer.PublicKeyAuth(func(ctx sshServer.Context, key sshServer.PublicKey) bool {
			allowed, _, _, _, _ := sshServer.ParseAuthorizedKey(a.pubKey)
			return sshServer.KeysEqual(key, allowed)
		}),
	); err != nil {
		log.Fatal(err)
	}
}

func (a *Agent) handleSession(s sshServer.Session) {
	stats := a.gatherStats()
	encoder := json.NewEncoder(s)
	if err := encoder.Encode(stats); err != nil {
		log.Println("Error encoding stats:", err.Error())
		s.Exit(1)
		return
	}
	s.Exit(0)
}

func (a *Agent) Run() {
	a.fsStats = make(map[string]*system.FsStats)

	// set sensors context (allows overriding sys location for sensors)
	if sysSensors, exists := os.LookupEnv("SYS_SENSORS"); exists {
		// log.Println("Using sys location for sensors:", sysSensors)
		a.sensorsContext = context.WithValue(a.sensorsContext,
			common.EnvKey, common.EnvMap{common.HostSysEnvKey: sysSensors},
		)
	}

	a.initializeDiskInfo()
	a.initializeDiskIoStats()
	a.initializeNetIoStats()

	// log.Printf("Filesystems: %+v\n", a.fsStats)
	a.startServer()
}

// Sets up the filesystems to monitor for disk usage and I/O.
func (a *Agent) initializeDiskInfo() error {
	filesystem := os.Getenv("FILESYSTEM")
	hasRoot := false

	// add values from EXTRA_FILESYSTEMS env var to fsStats
	if extraFilesystems, exists := os.LookupEnv("EXTRA_FILESYSTEMS"); exists {
		for _, filesystem := range strings.Split(extraFilesystems, ",") {
			a.fsStats[filepath.Base(filesystem)] = &system.FsStats{}
		}
	}

	partitions, err := disk.Partitions(false)
	if err != nil {
		return err
	}

	// if FILESYSTEM env var is set, use it to find root filesystem
	if filesystem != "" {
		for _, v := range partitions {
			// use filesystem env var if matching partition is found
			if strings.HasSuffix(v.Device, filesystem) || v.Mountpoint == filesystem {
				a.fsStats[filepath.Base(v.Device)] = &system.FsStats{Root: true, Mountpoint: v.Mountpoint}
				hasRoot = true
				break
			}
		}
		if !hasRoot {
			// if no match, log available partition details
			log.Printf("Partition details not found for %s:\n", filesystem)
			for _, v := range partitions {
				fmt.Printf("%+v\n", v)
			}
		}
	}

	for _, v := range partitions {
		// binary root fallback - use root mountpoint
		if !hasRoot && v.Mountpoint == "/" {
			a.fsStats[filepath.Base(v.Device)] = &system.FsStats{Root: true, Mountpoint: "/"}
			hasRoot = true
		}
		// docker root fallback - use /etc/hosts device if not mapped
		if !hasRoot && v.Mountpoint == "/etc/hosts" && strings.HasPrefix(v.Device, "/dev") && !strings.Contains(v.Device, "mapper") {
			a.fsStats[filepath.Base(v.Device)] = &system.FsStats{Root: true, Mountpoint: "/"}
			hasRoot = true
		}
		// check if device is in /extra-filesystem
		if strings.HasPrefix(v.Mountpoint, "/extra-filesystem") {
			// add to fsStats if not already there
			if _, exists := a.fsStats[filepath.Base(v.Device)]; !exists {
				a.fsStats[filepath.Base(v.Device)] = &system.FsStats{Mountpoint: v.Mountpoint}
			}
			continue
		}
		// set mountpoints for extra filesystems if passed in via env var
		for name, stats := range a.fsStats {
			if strings.HasSuffix(v.Device, name) {
				stats.Mountpoint = v.Mountpoint
				break
			}
		}
	}

	// remove extra filesystems that don't have a mountpoint
	for name, stats := range a.fsStats {
		if stats.Root {
			log.Println("Detected root fs:", name)
		}
		if stats.Mountpoint == "" {
			log.Printf("Ignoring %s. No mountpoint found.\n", name)
			delete(a.fsStats, name)
		}
	}

	// if no root filesystem set, use most read device in /proc/diskstats
	if !hasRoot {
		rootDevice := findFallbackIoDevice(filepath.Base(filesystem))
		log.Printf("Using / as mountpoint and %s for I/O\n", rootDevice)
		a.fsStats[rootDevice] = &system.FsStats{Root: true, Mountpoint: "/"}
	}

	return nil
}

// Sets start values for disk I/O stats.
func (a *Agent) initializeDiskIoStats() {
	// create slice of fs names to pass to disk.IOCounters
	a.fsNames = make([]string, 0, len(a.fsStats))
	for name := range a.fsStats {
		a.fsNames = append(a.fsNames, name)
	}

	if ioCounters, err := disk.IOCounters(a.fsNames...); err == nil {
		for _, d := range ioCounters {
			if a.fsStats[d.Name] == nil {
				continue
			}
			a.fsStats[d.Name].Time = time.Now()
			a.fsStats[d.Name].TotalRead = d.ReadBytes
			a.fsStats[d.Name].TotalWrite = d.WriteBytes
		}
	}
}

func (a *Agent) initializeNetIoStats() {
	// reset valid network interfaces
	a.netInterfaces = make(map[string]struct{}, 0)

	// map of network interface names passed in via NICS env var
	var nicsMap map[string]struct{}
	nics, nicsEnvExists := os.LookupEnv("NICS")
	if nicsEnvExists {
		nicsMap = make(map[string]struct{}, 0)
		for _, nic := range strings.Split(nics, ",") {
			nicsMap[nic] = struct{}{}
		}
	}

	// reset network I/O stats
	a.netIoStats.BytesSent = 0
	a.netIoStats.BytesRecv = 0

	// get intial network I/O stats
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		a.netIoStats.Time = time.Now()
		for _, v := range netIO {
			switch {
			// skip if nics exists and the interface is not in the list
			case nicsEnvExists:
				if _, nameInNics := nicsMap[v.Name]; !nameInNics {
					continue
				}
			// otherwise run the interface name through the skipNetworkInterface function
			default:
				if a.skipNetworkInterface(v) {
					continue
				}
			}
			log.Printf("Detected network interface: %+v (%+v recv, %+v sent)\n", v.Name, v.BytesRecv, v.BytesSent)
			a.netIoStats.BytesSent += v.BytesSent
			a.netIoStats.BytesRecv += v.BytesRecv
			// store as a valid network interface
			a.netInterfaces[v.Name] = struct{}{}
		}
	}
}

func bytesToMegabytes(b float64) float64 {
	return twoDecimals(b / 1048576)
}

func bytesToGigabytes(b uint64) float64 {
	return twoDecimals(float64(b) / 1073741824)
}

func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}

func (a *Agent) skipNetworkInterface(v psutilNet.IOCountersStat) bool {
	switch {
	case strings.HasPrefix(v.Name, "lo"),
		strings.HasPrefix(v.Name, "docker"),
		strings.HasPrefix(v.Name, "br-"),
		strings.HasPrefix(v.Name, "veth"),
		v.BytesRecv == 0,
		v.BytesSent == 0:
		return true
	default:
		return false
	}
}

func newDockerClient() *http.Client {
	dockerHost := "unix:///var/run/docker.sock"
	if dockerHostEnv, exists := os.LookupEnv("DOCKER_HOST"); exists {
		dockerHost = dockerHostEnv
	}

	parsedURL, err := url.Parse(dockerHost)
	if err != nil {
		log.Fatal("Error parsing DOCKER_HOST: " + err.Error())
	}

	transport := &http.Transport{
		ForceAttemptHTTP2:   false,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		MaxConnsPerHost:     20,
		MaxIdleConnsPerHost: 20,
		DisableKeepAlives:   false,
	}

	switch parsedURL.Scheme {
	case "unix":
		transport.DialContext = func(ctx context.Context, proto, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", parsedURL.Path)
		}
	case "tcp", "http", "https":
		log.Println("Using DOCKER_HOST: " + dockerHost)
		transport.DialContext = func(ctx context.Context, proto, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", parsedURL.Host)
		}
	default:
		log.Fatal("Unsupported DOCKER_HOST: " + parsedURL.Scheme)
	}

	return &http.Client{
		Timeout:   time.Second,
		Transport: transport,
	}
}

// closes idle connections on timeouts to prevent reuse of stale connections
func (a *Agent) closeIdleConnections(err error) (isTimeout bool) {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		log.Printf("Closing idle connections. Error: %+v\n", err)
		a.dockerClient.Transport.(*http.Transport).CloseIdleConnections()
		return true
	}
	return false
}

// Returns the device with the most reads in /proc/diskstats,
// or the device specified by the filesystem argument if it exists
// (fallback in case the root device is not supplied or detected)
func findFallbackIoDevice(filesystem string) string {
	var maxReadBytes uint64
	maxReadDevice := "/"
	counters, err := disk.IOCounters()
	if err != nil {
		return maxReadDevice
	}
	for _, d := range counters {
		if d.Name == filesystem {
			return d.Name
		}
		if d.ReadBytes > maxReadBytes {
			maxReadBytes = d.ReadBytes
			maxReadDevice = d.Name
		}
	}
	return maxReadDevice
}
