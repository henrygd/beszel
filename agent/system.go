package agent

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent/battery"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/jaypipes/ghw/pkg/block"
	ghwnet "github.com/jaypipes/ghw/pkg/net"
	ghwpci "github.com/jaypipes/ghw/pkg/pci"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
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
		a.systemInfo.Os = system.Windows
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
		slog.Debug("CPU info populated", "cpus", a.systemInfo.Cpus)
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
		slog.Debug("Memory info populated", "memory", a.systemInfo.Memory)
	}

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
func (a *Agent) getSystemStats(cacheTimeMs uint16) system.Stats {
	var systemStats system.Stats

	// battery
	if batteryPercent, batteryState, err := battery.GetBatteryStats(); err == nil {
		systemStats.Battery[0] = batteryPercent
		systemStats.Battery[1] = batteryState
	}

	// cpu percent
	cpuPercent, err := getCpuPercent(cacheTimeMs)
	if err == nil {
		systemStats.Cpu = twoDecimals(cpuPercent)
	} else {
		slog.Error("Error getting cpu percent", "err", err)
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

