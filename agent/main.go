package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	sshServer "github.com/gliderlabs/ssh"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

var Version = "0.1.0"

var containerCpuMap = make(map[string][2]uint64)
var containerCpuMutex = &sync.Mutex{}

var sem = make(chan struct{}, 15)

func acquireSemaphore() {
	sem <- struct{}{}
}

func releaseSemaphore() {
	<-sem
}

var diskIoStats = DiskIoStats{
	Read:       0,
	Write:      0,
	Time:       time.Now(),
	Filesystem: "",
}

var netIoStats = NetIoStats{
	BytesRecv: 0,
	BytesSent: 0,
	Time:      time.Now(),
	Name:      "",
}

// client for docker engine api
var client = &http.Client{
	Timeout: time.Second,
	Transport: &http.Transport{
		Dial: func(proto, addr string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
		ForceAttemptHTTP2:   false,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 50,
		DisableKeepAlives:   false,
	},
}

func getSystemStats() (*SystemInfo, *SystemStats) {
	systemStats := &SystemStats{}

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
	}

	// disk usage
	if d, err := disk.Usage("/"); err == nil {
		systemStats.Disk = bytesToGigabytes(d.Total)
		systemStats.DiskUsed = bytesToGigabytes(d.Used)
		systemStats.DiskPct = twoDecimals(d.UsedPercent)
	}

	// disk i/o
	if io, err := disk.IOCounters(diskIoStats.Filesystem); err == nil {
		for _, d := range io {
			// add to systemStats
			secondsElapsed := time.Since(diskIoStats.Time).Seconds()
			readPerSecond := float64(d.ReadBytes-diskIoStats.Read) / secondsElapsed
			systemStats.DiskRead = bytesToMegabytes(readPerSecond)
			writePerSecond := float64(d.WriteBytes-diskIoStats.Write) / secondsElapsed
			systemStats.DiskWrite = bytesToMegabytes(writePerSecond)
			// update diskIoStats
			diskIoStats.Time = time.Now()
			diskIoStats.Read = d.ReadBytes
			diskIoStats.Write = d.WriteBytes
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
		secondsElapsed := time.Since(netIoStats.Time).Seconds()
		sentPerSecond := float64(bytesSent-netIoStats.BytesSent) / secondsElapsed
		recvPerSecond := float64(bytesRecv-netIoStats.BytesRecv) / secondsElapsed
		systemStats.NetworkSent = bytesToMegabytes(sentPerSecond)
		systemStats.NetworkRecv = bytesToMegabytes(recvPerSecond)
		// update netIoStats
		netIoStats.BytesSent = bytesSent
		netIoStats.BytesRecv = bytesRecv
		netIoStats.Time = time.Now()
	}

	systemInfo := &SystemInfo{
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

func getDockerStats() ([]*ContainerStats, error) {
	resp, err := client.Get("http://localhost/containers/json")
	if err != nil {
		return []*ContainerStats{}, err
	}
	defer resp.Body.Close()

	var containers []*Container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		panic(err)
	}

	containerStats := make([]*ContainerStats, 0, len(containers))

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
			delete(containerCpuMap, ctr.IdShort)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			cstats, err := getContainerStats(ctr)
			if err != nil {
				// delete container from map and retry once
				delete(containerCpuMap, ctr.IdShort)
				cstats, err = getContainerStats(ctr)
				if err != nil {
					log.Printf("Error getting container stats: %+v\n", err)
					return
				}
			}
			containerStats = append(containerStats, cstats)
		}()
	}

	wg.Wait()

	for id := range containerCpuMap {
		if _, exists := validIds[id]; !exists {
			// log.Printf("Removing container cpu map entry: %+v\n", id)
			delete(containerCpuMap, id)
		}
	}

	return containerStats, nil
}

func getContainerStats(ctr *Container) (*ContainerStats, error) {
	// use semaphore to limit concurrency
	acquireSemaphore()
	defer releaseSemaphore()
	resp, err := client.Get("http://localhost/containers/" + ctr.IdShort + "/stats?stream=0&one-shot=1")
	if err != nil {
		return &ContainerStats{}, err
	}
	defer resp.Body.Close()

	var statsJson CStats
	if err := json.NewDecoder(resp.Body).Decode(&statsJson); err != nil {
		panic(err)
	}

	name := ctr.Names[0][1:]

	// memory (https://docs.docker.com/reference/cli/docker/container/stats/)
	memCache := statsJson.MemoryStats.Stats["inactive_file"]
	if memCache == 0 {
		memCache = statsJson.MemoryStats.Stats["cache"]
	}
	usedMemory := statsJson.MemoryStats.Usage - memCache
	// pctMemory := float64(usedMemory) / float64(statsJson.MemoryStats.Limit) * 100

	// cpu
	// add default values to containerCpu if it doesn't exist
	containerCpuMutex.Lock()
	defer containerCpuMutex.Unlock()
	if _, ok := containerCpuMap[ctr.IdShort]; !ok {
		containerCpuMap[ctr.IdShort] = [2]uint64{0, 0}
	}
	cpuDelta := statsJson.CPUStats.CPUUsage.TotalUsage - containerCpuMap[ctr.IdShort][0]
	systemDelta := statsJson.CPUStats.SystemUsage - containerCpuMap[ctr.IdShort][1]
	cpuPct := float64(cpuDelta) / float64(systemDelta) * 100
	if cpuPct > 100 {
		return &ContainerStats{}, fmt.Errorf("%s cpu pct greater than 100: %+v", name, cpuPct)
	}
	containerCpuMap[ctr.IdShort] = [2]uint64{statsJson.CPUStats.CPUUsage.TotalUsage, statsJson.CPUStats.SystemUsage}

	cStats := &ContainerStats{
		Name: name,
		Cpu:  twoDecimals(cpuPct),
		Mem:  bytesToMegabytes(float64(usedMemory)),
		// MemPct: twoDecimals(pctMemory),
	}
	return cStats, nil
}

func gatherStats() *SystemData {
	systemInfo, systemStats := getSystemStats()
	stats := &SystemData{
		Stats:      systemStats,
		Info:       systemInfo,
		Containers: []*ContainerStats{},
	}
	containerStats, err := getDockerStats()
	if err == nil {
		stats.Containers = containerStats
	}
	// fmt.Printf("%+v\n", stats)
	return stats
}

func startServer(addr string, pubKey []byte) {
	sshServer.Handle(func(s sshServer.Session) {
		stats := gatherStats()
		var jsonStats []byte
		jsonStats, _ = json.Marshal(stats)
		io.WriteString(s, string(jsonStats))
		s.Exit(0)
	})

	log.Printf("Starting SSH server on %s", addr)
	if err := sshServer.ListenAndServe(addr, nil, sshServer.NoPty(),
		sshServer.PublicKeyAuth(func(ctx sshServer.Context, key sshServer.PublicKey) bool {
			data := []byte(pubKey)
			allowed, _, _, _, _ := sshServer.ParseAuthorizedKey(data)
			return sshServer.KeysEqual(key, allowed)
		}),
	); err != nil {
		log.Fatal(err)
	}
}

func main() {
	// handle flags / subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v":
			fmt.Println("beszel-agent", Version)
		case "update":
			updateBeszel()
		}
		os.Exit(0)
	}

	var pubKey []byte
	if pubKeyEnv, exists := os.LookupEnv("KEY"); exists {
		pubKey = []byte(pubKeyEnv)
	} else {
		log.Fatal("KEY environment variable is not set")
	}

	if filesystem, exists := os.LookupEnv("FILESYSTEM"); exists {
		diskIoStats.Filesystem = filesystem
	} else {
		diskIoStats.Filesystem = findDefaultFilesystem()
	}

	initializeDiskIoStats()
	initializeNetIoStats()

	if port, exists := os.LookupEnv("PORT"); exists {
		// allow passing an address in the form of "127.0.0.1:45876"
		if !strings.Contains(port, ":") {
			port = ":" + port
		}
		startServer(port, pubKey)
	} else {
		startServer(":45876", pubKey)
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

func initializeDiskIoStats() {
	if io, err := disk.IOCounters(diskIoStats.Filesystem); err == nil {
		for _, d := range io {
			diskIoStats.Time = time.Now()
			diskIoStats.Read = d.ReadBytes
			diskIoStats.Write = d.WriteBytes
		}
	}
}

func initializeNetIoStats() {
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
		netIoStats.BytesSent = bytesSent
		netIoStats.BytesRecv = bytesRecv
		netIoStats.Time = time.Now()
	}
}
