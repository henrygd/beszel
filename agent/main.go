package main

import (
	"encoding/json"
	"errors"
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

var Version = "0.0.1-alpha.3"

var containerCpuMap = make(map[string][2]uint64)
var containerCpuMutex = &sync.Mutex{}

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
	Timeout: time.Second * 5,
	Transport: &http.Transport{
		Dial: func(proto, addr string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
		ForceAttemptHTTP2:   false,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	},
}

type SystemData struct {
	Stats      SystemStats      `json:"stats"`
	Info       SystemInfo       `json:"info"`
	Containers []ContainerStats `json:"container"`
}

type SystemInfo struct {
	Cores    int    `json:"c"`
	Threads  int    `json:"t"`
	CpuModel string `json:"m"`
	// Os       string  `json:"o"`
	Uptime  uint64  `json:"u"`
	Cpu     float64 `json:"cpu"`
	MemPct  float64 `json:"mp"`
	DiskPct float64 `json:"dp"`
}

type SystemStats struct {
	Cpu          float64 `json:"cpu"`
	Mem          float64 `json:"m"`
	MemUsed      float64 `json:"mu"`
	MemPct       float64 `json:"mp"`
	MemBuffCache float64 `json:"mb"`
	Disk         float64 `json:"d"`
	DiskUsed     float64 `json:"du"`
	DiskPct      float64 `json:"dp"`
	DiskRead     float64 `json:"dr"`
	DiskWrite    float64 `json:"dw"`
	NetworkSent  float64 `json:"ns"`
	NetworkRecv  float64 `json:"nr"`
}

type ContainerStats struct {
	Name string  `json:"n"`
	Cpu  float64 `json:"c"`
	Mem  float64 `json:"m"`
	// MemPct float64 `json:"mp"`
}

func getSystemStats() (SystemInfo, SystemStats) {
	c, _ := cpu.Percent(0, false)
	v, _ := mem.VirtualMemory()
	d, _ := disk.Usage("/")

	cpuPct := twoDecimals(c[0])
	memPct := twoDecimals(v.UsedPercent)
	diskPct := twoDecimals(d.UsedPercent)

	systemStats := SystemStats{
		Cpu:          cpuPct,
		Mem:          bytesToGigabytes(v.Total),
		MemUsed:      bytesToGigabytes(v.Used),
		MemBuffCache: bytesToGigabytes(v.Total - v.Free - v.Used),
		MemPct:       memPct,
		Disk:         bytesToGigabytes(d.Total),
		DiskUsed:     bytesToGigabytes(d.Used),
		DiskPct:      diskPct,
	}

	systemInfo := SystemInfo{
		Cpu:     cpuPct,
		MemPct:  memPct,
		DiskPct: diskPct,
	}

	// add disk stats
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

	// add network stats
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		bytesSent := uint64(0)
		bytesRecv := uint64(0)
		for _, v := range netIO {
			if skipNetworkInterface(v.Name) {
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

	// add host stats
	if info, err := host.Info(); err == nil {
		systemInfo.Uptime = info.Uptime
		// systemInfo.Os = info.OS
	}
	// add cpu stats
	if info, err := cpu.Info(); err == nil {
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

func getDockerStats() ([]ContainerStats, error) {
	resp, err := client.Get("http://localhost/containers/json")
	if err != nil {
		return []ContainerStats{}, err
	}
	defer resp.Body.Close()

	var containers []Container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	var containerStats []ContainerStats

	for _, ctr := range containers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cstats, err := getContainerStats(ctr)
			if err != nil {
				log.Printf("Error getting container stats: %+v\n", err)
				return
			}
			containerStats = append(containerStats, cstats)
		}()
	}

	// clean up old containers from map
	validNames := make(map[string]struct{}, len(containers))
	for _, ctr := range containers {
		validNames[ctr.Names[0][1:]] = struct{}{}
	}
	for name := range containerCpuMap {
		if _, exists := validNames[name]; !exists {
			delete(containerCpuMap, name)
		}
	}

	wg.Wait()

	return containerStats, nil
}

func getContainerStats(ctr Container) (ContainerStats, error) {
	resp, err := client.Get("http://localhost/containers/" + ctr.ID + "/stats?stream=0&one-shot=1")
	if err != nil {
		return ContainerStats{}, err
	}
	defer resp.Body.Close()

	var statsJson CStats
	if err := json.NewDecoder(resp.Body).Decode(&statsJson); err != nil {
		panic(err)
	}

	name := ctr.Names[0][1:]

	// memory
	usedMemory := statsJson.MemoryStats.Usage - statsJson.MemoryStats.Cache
	// pctMemory := float64(usedMemory) / float64(statsJson.MemoryStats.Limit) * 100

	// cpu
	// add default values to containerCpu if it doesn't exist
	containerCpuMutex.Lock()
	defer containerCpuMutex.Unlock()
	if _, ok := containerCpuMap[name]; !ok {
		containerCpuMap[name] = [2]uint64{0, 0}
	}
	cpuDelta := statsJson.CPUStats.CPUUsage.TotalUsage - containerCpuMap[name][0]
	systemDelta := statsJson.CPUStats.SystemUsage - containerCpuMap[name][1]
	cpuPct := float64(cpuDelta) / float64(systemDelta) * 100
	if cpuPct > 100 {
		return ContainerStats{}, errors.New("cpu pct is greater than 100")
	}
	containerCpuMap[name] = [2]uint64{statsJson.CPUStats.CPUUsage.TotalUsage, statsJson.CPUStats.SystemUsage}

	cStats := ContainerStats{
		Name: name,
		Cpu:  twoDecimals(cpuPct),
		Mem:  bytesToMegabytes(float64(usedMemory)),
		// MemPct: twoDecimals(pctMemory),
	}
	return cStats, nil
}

func gatherStats() SystemData {
	systemInfo, systemStats := getSystemStats()
	stats := SystemData{
		Stats:      systemStats,
		Info:       systemInfo,
		Containers: []ContainerStats{},
	}
	containerStats, err := getDockerStats()
	if err == nil {
		stats.Containers = containerStats
	}
	// fmt.Printf("%+v\n", stats)
	return stats
}

func startServer(port string, pubKey []byte) {
	sshServer.Handle(func(s sshServer.Session) {
		stats := gatherStats()
		var jsonStats []byte
		jsonStats, _ = json.Marshal(stats)
		io.WriteString(s, string(jsonStats))
		s.Exit(0)
	})

	log.Printf("Starting SSH server on port %s", port)
	if err := sshServer.ListenAndServe(":"+port, nil, sshServer.NoPty(),
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
		pubKey = []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJgPK8kpPOwPFIq6BIa7Bu/xwrjt5VRQCz3az3Glt4jp")
		// log.Fatal("KEY environment variable is not set")
	}

	if filesystem, exists := os.LookupEnv("FILESYSTEM"); exists {
		diskIoStats.Filesystem = filesystem
	} else {
		diskIoStats.Filesystem = findDefaultFilesystem()
	}

	initializeDiskIoStats()
	initializeNetIoStats()

	if port, exists := os.LookupEnv("PORT"); exists {
		startServer(port, pubKey)
	} else {
		startServer("45876", pubKey)
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

func skipNetworkInterface(name string) bool {
	return strings.HasPrefix(name, "lo") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "veth")
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
			if skipNetworkInterface(v.Name) {
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
