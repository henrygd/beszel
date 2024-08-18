package agent

import (
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
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"

	sshServer "github.com/gliderlabs/ssh"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

type Agent struct {
	addr                string
	pubKey              []byte
	sem                 chan struct{}
	containerStatsMap   map[string]*container.PrevContainerStats
	containerStatsMutex *sync.Mutex
	diskIoStats         *system.DiskIoStats
	netIoStats          *system.NetIoStats
	dockerClient        *http.Client
	containerStatsPool  *sync.Pool
	bufferPool          *sync.Pool
}

func NewAgent(pubKey []byte, addr string) *Agent {
	return &Agent{
		addr:                addr,
		pubKey:              pubKey,
		sem:                 make(chan struct{}, 15),
		containerStatsMap:   make(map[string]*container.PrevContainerStats),
		containerStatsMutex: &sync.Mutex{},
		diskIoStats:         &system.DiskIoStats{},
		netIoStats:          &system.NetIoStats{},
		dockerClient:        newDockerClient(),
		containerStatsPool: &sync.Pool{
			New: func() interface{} {
				return new(container.Stats)
			},
		},
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (a *Agent) acquireSemaphore() {
	a.sem <- struct{}{}
}

func (a *Agent) releaseSemaphore() {
	<-a.sem
}

func (a *Agent) getSystemStats() (*system.Info, *system.Stats) {
	systemStats := &system.Stats{}

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
	if d, err := disk.Usage("/"); err == nil {
		systemStats.Disk = bytesToGigabytes(d.Total)
		systemStats.DiskUsed = bytesToGigabytes(d.Used)
		systemStats.DiskPct = twoDecimals(d.UsedPercent)
	}

	// disk i/o
	if io, err := disk.IOCounters(a.diskIoStats.Filesystem); err == nil {
		for _, d := range io {
			// add to systemStats
			secondsElapsed := time.Since(a.diskIoStats.Time).Seconds()
			readPerSecond := float64(d.ReadBytes-a.diskIoStats.Read) / secondsElapsed
			systemStats.DiskRead = bytesToMegabytes(readPerSecond)
			writePerSecond := float64(d.WriteBytes-a.diskIoStats.Write) / secondsElapsed
			systemStats.DiskWrite = bytesToMegabytes(writePerSecond)
			// update diskIoStats
			a.diskIoStats.Time = time.Now()
			a.diskIoStats.Read = d.ReadBytes
			a.diskIoStats.Write = d.WriteBytes
		}
	}

	// network stats
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		bytesSent := uint64(0)
		bytesRecv := uint64(0)
		for _, v := range netIO {
			if skipNetworkInterface(&v) {
				continue
			}
			// log.Printf("%+v: %+v recv, %+v sent\n", v.Name, v.BytesRecv, v.BytesSent)
			bytesSent += v.BytesSent
			bytesRecv += v.BytesRecv
		}
		// add to systemStats
		secondsElapsed := time.Since(a.netIoStats.Time).Seconds()
		sentPerSecond := float64(bytesSent-a.netIoStats.BytesSent) / secondsElapsed
		recvPerSecond := float64(bytesRecv-a.netIoStats.BytesRecv) / secondsElapsed
		systemStats.NetworkSent = bytesToMegabytes(sentPerSecond)
		systemStats.NetworkRecv = bytesToMegabytes(recvPerSecond)
		// update netIoStats
		a.netIoStats.BytesSent = bytesSent
		a.netIoStats.BytesRecv = bytesRecv
		a.netIoStats.Time = time.Now()
	}

	systemInfo := &system.Info{
		Cpu:     systemStats.Cpu,
		MemPct:  systemStats.MemPct,
		DiskPct: systemStats.DiskPct,
	}

	// add host info
	if info, err := host.Info(); err == nil {
		systemInfo.Uptime = info.Uptime
		// systemInfo.Os = info.OS
	}
	// add cpu stats
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		systemInfo.CpuModel = info[0].ModelName
	}
	if cores, err := cpu.Counts(false); err == nil {
		systemInfo.Cores = cores
	}
	if threads, err := cpu.Counts(true); err == nil {
		systemInfo.Threads = threads
	}

	return systemInfo, systemStats

}

func (a *Agent) getDockerStats() ([]*container.Stats, error) {
	resp, err := a.dockerClient.Get("http://localhost/containers/json")
	if err != nil {
		a.closeIdleConnections(err)
		return nil, err
	}
	defer resp.Body.Close()

	var containers []*container.ApiInfo
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		log.Printf("Error decoding containers: %+v\n", err)
		return nil, err
	}

	containerStats := make([]*container.Stats, 0, len(containers))

	// store valid ids to clean up old container ids from map
	validIds := make(map[string]struct{}, len(containers))

	var wg sync.WaitGroup

	for _, ctr := range containers {
		ctr.IdShort = ctr.Id[:12]
		validIds[ctr.IdShort] = struct{}{}
		// check if container is less than 1 minute old (possible restart)
		// note: can't use Created field because it's not updated on restart
		if strings.HasSuffix(ctr.Status, "seconds") {
			// if so, remove old container data
			a.deleteContainerStatsSync(ctr.IdShort)
		}
		wg.Add(1)
		go func() {
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

func (a *Agent) getContainerStats(ctr *container.ApiInfo) (*container.Stats, error) {
	// use semaphore to limit concurrency
	a.acquireSemaphore()
	defer a.releaseSemaphore()

	resp, err := a.dockerClient.Get("http://localhost/containers/" + ctr.IdShort + "/stats?stream=0&one-shot=1")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// get a buffer from the pool
	buf := a.bufferPool.Get().(*bytes.Buffer)
	defer a.bufferPool.Put(buf)
	buf.Reset()
	// read the response body into the buffer
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return nil, err
	}

	// unmarshal the json data from the buffer
	var statsJson container.ApiStats
	if err := json.Unmarshal(buf.Bytes(), &statsJson); err != nil {
		return nil, err
	}

	name := ctr.Names[0][1:]

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
		return nil, fmt.Errorf("%s cpu pct greater than 100: %+v", name, cpuPct)
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

	cStats := a.containerStatsPool.Get().(*container.Stats)
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

func (a *Agent) gatherStats() *system.CombinedData {
	systemInfo, systemStats := a.getSystemStats()
	systemData := &system.CombinedData{
		Stats: systemStats,
		Info:  systemInfo,
	}
	if containerStats, err := a.getDockerStats(); err == nil {
		systemData.Containers = containerStats
	}
	// fmt.Printf("%+v\n", systemData)
	return systemData
}

// return container stats to pool
func (a *Agent) returnStatsToPool(containerStats []*container.Stats) {
	for _, stats := range containerStats {
		a.containerStatsPool.Put(stats)
	}
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
	defer a.returnStatsToPool(stats.Containers)
	encoder := json.NewEncoder(s)
	if err := encoder.Encode(stats); err != nil {
		log.Println("Error encoding stats:", err.Error())
		s.Exit(1)
		return
	}
	s.Exit(0)
}

func (a *Agent) Run() {
	if filesystem, exists := os.LookupEnv("FILESYSTEM"); exists {
		a.diskIoStats.Filesystem = filesystem
	} else {
		a.diskIoStats.Filesystem = findDefaultFilesystem()
	}

	a.initializeDiskIoStats()
	a.initializeNetIoStats()

	a.startServer()
}

func (a *Agent) initializeDiskIoStats() {
	if io, err := disk.IOCounters(a.diskIoStats.Filesystem); err == nil {
		for _, d := range io {
			a.diskIoStats.Time = time.Now()
			a.diskIoStats.Read = d.ReadBytes
			a.diskIoStats.Write = d.WriteBytes
		}
	}
}

func (a *Agent) initializeNetIoStats() {
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		bytesSent := uint64(0)
		bytesRecv := uint64(0)
		for _, v := range netIO {
			if skipNetworkInterface(&v) {
				continue
			}
			log.Printf("Found network interface: %+v (%+v recv, %+v sent)\n", v.Name, v.BytesRecv, v.BytesSent)
			bytesSent += v.BytesSent
			bytesRecv += v.BytesRecv
		}
		a.netIoStats.BytesSent = bytesSent
		a.netIoStats.BytesRecv = bytesRecv
		a.netIoStats.Time = time.Now()
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

func findDefaultFilesystem() string {
	if partitions, err := disk.Partitions(false); err == nil {
		for _, v := range partitions {
			if v.Mountpoint == "/" {
				log.Printf("Using filesystem: %+v\n", v.Device)
				return v.Device
			}
		}
	}
	return ""
}

func skipNetworkInterface(v *psutilNet.IOCountersStat) bool {
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
