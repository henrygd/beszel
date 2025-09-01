package agent

import (
	"beszel"
	"beszel/internal/agent/battery"
	"beszel/internal/entities/system"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jaypipes/ghw/pkg/block"
	ghwnet "github.com/jaypipes/ghw/pkg/net"
	ghwpci "github.com/jaypipes/ghw/pkg/pci"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// GeoJSResponse represents the response from GeoJS API
type GeoJSResponse struct {
	IP               string `json:"ip"`
	OrganizationName string `json:"organization_name"`
	ASN              int    `json:"asn"`
}

// Sets initial / non-changing values about the host system
func (a *Agent) initializeSystemInfo() {
	a.systemInfo.AgentVersion = beszel.Version
	a.systemInfo.Hostname, _ = os.Hostname()

	platform, family, version, _ := host.PlatformInformation()

	var osFamily, osVersion, osKernel string
	if platform == "darwin" {
		osKernel = version
		osFamily = "macOS" // macOS is the family name for Darwin
		osVersion = version
	} else if strings.Contains(platform, "indows") {
		osKernel = strings.Replace(platform, "Microsoft ", "", 1) + " " + version
		osFamily = family
		osVersion = version
	} else if platform == "freebsd" {
		osKernel = version
		osFamily = family
		osVersion = version
	} else {
		osFamily = family
		osVersion = version
		osKernel = ""
		osRelease := readOsRelease()
		if pretty, ok := osRelease["PRETTY_NAME"]; ok {
			osFamily = pretty
		}
		if name, ok := osRelease["NAME"]; ok {
			osFamily = name
		}
		if versionId, ok := osRelease["VERSION_ID"]; ok {
			osVersion = versionId
		}
	}
	if osKernel == "" {
		osKernel, _ = host.KernelVersion()
	}
	a.systemInfo.Oses = []system.OsInfo{{
		Family:  osFamily,
		Version: osVersion,
		Kernel:  osKernel,
	}}

	// cpu model
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		arch := runtime.GOARCH
		totalCores := 0
		totalThreads := 0
		for _, cpuInfo := range info {
			totalCores += int(cpuInfo.Cores)
			totalThreads++
		}
		modelName := info[0].ModelName
		if idx := strings.Index(modelName, "@"); idx > 0 {
			modelName = strings.TrimSpace(modelName[:idx])
		}
		cpu := system.CpuInfo{
			Model:    modelName,
			SpeedGHz: fmt.Sprintf("%.2f GHz", info[0].Mhz/1000),
			Arch:     arch,
			Cores:    totalCores,
			Threads:  totalThreads,
		}
		a.systemInfo.Cpus = []system.CpuInfo{cpu}
	}

	// zfs
	if _, err := getARCSize(); err != nil {
		slog.Debug("Not monitoring ZFS ARC", "err", err)
	} else {
		a.zfs = true
	}

	// Collect disk info (model/vendor)
	a.systemInfo.Disks = getDiskInfo()

	// Collect network interface info
	a.systemInfo.Networks = getNetworkInfo()

	// Collect total memory and store in systemInfo.Memory
	if v, err := mem.VirtualMemory(); err == nil {
		total := fmt.Sprintf("%d GB", int((float64(v.Total)/(1024*1024*1024))+0.5))
		a.systemInfo.Memory = []system.MemoryInfo{{Total: total}}
	}

	// Collect IP, ISP, and ASN information
	a.getIPInfo()
}

// readPrettyName reads the PRETTY_NAME from /etc/os-release
func readPrettyName() string {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			// Remove the prefix and any surrounding quotes
			prettyName := strings.TrimPrefix(line, "PRETTY_NAME=")
			prettyName = strings.Trim(prettyName, `"`)
			return prettyName
		}
	}
	return ""
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
		// maybe refactor this in the future to not cache interface names at all so we
		// don't miss an interface that's been added after agent started in any circumstance
		a.initializeNetIoStats()
	}
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		msElapsed := uint64(time.Since(a.netIoStats.Time).Milliseconds())
		a.netIoStats.Time = time.Now()
		totalBytesSent := uint64(0)
		totalBytesRecv := uint64(0)
		// sum all bytes sent and received
		for _, v := range netIO {
			// skip if not in valid network interfaces list
			if _, exists := a.netInterfaces[v.Name]; !exists {
				continue
			}
			totalBytesSent += v.BytesSent
			totalBytesRecv += v.BytesRecv
		}
		// add to systemStats
		var bytesSentPerSecond, bytesRecvPerSecond uint64
		if msElapsed > 0 {
			bytesSentPerSecond = (totalBytesSent - a.netIoStats.BytesSent) * 1000 / msElapsed
			bytesRecvPerSecond = (totalBytesRecv - a.netIoStats.BytesRecv) * 1000 / msElapsed
		}
		networkSentPs := bytesToMegabytes(float64(bytesSentPerSecond))
		networkRecvPs := bytesToMegabytes(float64(bytesRecvPerSecond))
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
			systemStats.Bandwidth[0], systemStats.Bandwidth[1] = bytesSentPerSecond, bytesRecvPerSecond
			// update netIoStats
			a.netIoStats.BytesSent = totalBytesSent
			a.netIoStats.BytesRecv = totalBytesRecv
		}
	}

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
	// TODO: in future release, remove MB bandwidth values in favor of bytes
	a.systemInfo.Bandwidth = twoDecimals(systemStats.NetworkSent + systemStats.NetworkRecv)
	a.systemInfo.BandwidthBytes = systemStats.Bandwidth[0] + systemStats.Bandwidth[1]
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

func getDiskInfo() []system.DiskInfo {
	blockInfo, err := block.New()
	if err != nil {
		slog.Debug("Failed to get block info with ghw", "err", err)
		return nil
	}

	var disks []system.DiskInfo
	for _, disk := range blockInfo.Disks {
		disks = append(disks, system.DiskInfo{
			Name:   disk.Name,
			Model:  disk.Model,
			Vendor: disk.Vendor,
		})
	}
	return disks
}

func getNetworkInfo() []system.NetworkInfo {
	netInfo, err := ghwnet.New()
	if err != nil {
		slog.Debug("Failed to get network info with ghw", "err", err)
		return nil
	}
	pciInfo, err := ghwpci.New()
	if err != nil {
		slog.Debug("Failed to get PCI info with ghw", "err", err)
	}

	var networks []system.NetworkInfo
	for _, nic := range netInfo.NICs {
		if nic.IsVirtual {
			continue
		}
		var vendor, model string
		if nic.PCIAddress != nil && pciInfo != nil {
			for _, dev := range pciInfo.Devices {
				if dev.Address == *nic.PCIAddress {
					if dev.Vendor != nil {
						vendor = dev.Vendor.Name
					}
					if dev.Product != nil {
						model = dev.Product.Name
					}
					break
				}
			}
		}

		networks = append(networks, system.NetworkInfo{
			Name:   nic.Name,
			Vendor: vendor,
			Model:  model,
		})
	}
	return networks
}

// getInterfaceCapabilitiesFromGhw uses ghw library to get interface capabilities
func getInterfaceCapabilitiesFromGhw(nic *ghwnet.NIC) string {
	// Use the speed information from ghw if available
	if nic.Speed != "" {
		return nic.Speed
	}

	// If no speed info from ghw, try to get interface type from name
	return getInterfaceTypeFromName(nic.Name)
}

// getInterfaceTypeFromName tries to determine interface type from name
func getInterfaceTypeFromName(ifaceName string) string {
	// Common interface naming patterns
	switch {
	case strings.HasPrefix(ifaceName, "eth"):
		return "Ethernet"
	case strings.HasPrefix(ifaceName, "en"):
		return "Ethernet"
	case strings.HasPrefix(ifaceName, "wlan"):
		return "WiFi"
	case strings.HasPrefix(ifaceName, "wl"):
		return "WiFi"
	case strings.HasPrefix(ifaceName, "usb"):
		return "USB"
	case strings.HasPrefix(ifaceName, "tun"):
		return "Tunnel"
	case strings.HasPrefix(ifaceName, "tap"):
		return "TAP"
	case strings.HasPrefix(ifaceName, "br"):
		return "Bridge"
	case strings.HasPrefix(ifaceName, "bond"):
		return "Bond"
	case strings.HasPrefix(ifaceName, "veth"):
		return "Virtual Ethernet"
	case strings.HasPrefix(ifaceName, "docker"):
		return "Docker"
	case strings.HasPrefix(ifaceName, "lo"):
		return "Loopback"
	default:
		return ""
	}
}

func readOsRelease() map[string]string {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return map[string]string{}
	}
	defer file.Close()

	release := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if i := strings.Index(line, "="); i > 0 {
			key := line[:i]
			val := strings.Trim(line[i+1:], `"`)
			release[key] = val
		}
	}
	return release
}

func getMemoryInfo() []system.MemoryInfo {
	var total string
	if v, err := mem.VirtualMemory(); err == nil {
		total = fmt.Sprintf("%d GB", int((float64(v.Total)/(1024*1024*1024))+0.5))
	}
	return []system.MemoryInfo{{
		Total: total,
	}}
}

// getIPInfo collects public IP, ISP, and ASN information using GeoJS API
func (a *Agent) getIPInfo() {
	resp, err := http.Get("https://get.geojs.io/v1/ip/geo.json")
	if err != nil {
		slog.Debug("Failed to get IP info", "err", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Debug("Failed to read IP info response", "err", err)
		return
	}

	var geoInfo GeoJSResponse
	if err := json.Unmarshal(body, &geoInfo); err != nil {
		slog.Debug("Failed to parse IP info response", "err", err)
		return
	}

	var asn string
	if geoInfo.ASN > 0 {
		asn = fmt.Sprintf("AS%d", geoInfo.ASN)
	}

	networkLoc := system.NetworkLocationInfo{
		PublicIP: geoInfo.IP,
		ISP:      geoInfo.OrganizationName,
		ASN:      asn,
	}
	a.systemInfo.NetworkLoc = []system.NetworkLocationInfo{networkLoc}

	slog.Debug("IP info collected", "ip", geoInfo.IP, "isp", geoInfo.OrganizationName, "asn", geoInfo.ASN)
}
