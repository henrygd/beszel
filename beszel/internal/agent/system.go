package agent

import (
	"beszel"
	"beszel/internal/entities/system"
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jaypipes/ghw/pkg/block"
	ghwmem "github.com/jaypipes/ghw/pkg/memory"
	ghwnet "github.com/jaypipes/ghw/pkg/net"
	ghwpci "github.com/jaypipes/ghw/pkg/pci"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// Sets initial / non-changing values about the host system
func (a *Agent) initializeSystemInfo() {
	a.systemInfo.AgentVersion = beszel.Version
	a.systemInfo.Hostname, _ = os.Hostname()

	platform, family, version, _ := host.PlatformInformation()

	if platform == "darwin" {
		a.systemInfo.KernelVersion = version
		a.systemInfo.Os = system.Darwin
		a.systemInfo.OsName = "macOS"
		a.systemInfo.OsVersion = version
		a.systemInfo.OsNameRaw = family
		a.systemInfo.OsVersionId = version
	} else if strings.Contains(platform, "indows") {
		a.systemInfo.KernelVersion = strings.Replace(platform, "Microsoft ", "", 1) + " " + version
		a.systemInfo.Os = system.Windows
		a.systemInfo.OsName = family
		a.systemInfo.OsVersion = version
		a.systemInfo.OsNameRaw = family
		a.systemInfo.OsVersionId = version
	} else if platform == "freebsd" {
		a.systemInfo.Os = system.Freebsd
		a.systemInfo.KernelVersion = version
		a.systemInfo.OsName = family
		a.systemInfo.OsVersion = version
		a.systemInfo.OsNameRaw = family
		a.systemInfo.OsVersionId = version
	} else {
		a.systemInfo.Os = system.Linux
		a.systemInfo.OsName = family
		a.systemInfo.OsVersion = version
		a.systemInfo.OsNameRaw = family
		a.systemInfo.OsVersionId = version

		// Use /etc/os-release for more accurate Linux OS info
		osRelease := readOsRelease()
		if pretty, ok := osRelease["PRETTY_NAME"]; ok {
			a.systemInfo.OsName = pretty
		}
		if name, ok := osRelease["NAME"]; ok {
			a.systemInfo.OsNameRaw = name
		}
		if versionId, ok := osRelease["VERSION_ID"]; ok {
			a.systemInfo.OsVersionId = versionId
		}
	}

	// Set OS architecture
	a.systemInfo.OsArch = runtime.GOARCH

	if a.systemInfo.KernelVersion == "" {
		a.systemInfo.KernelVersion, _ = host.KernelVersion()
	}

	// cpu model
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		modelName := info[0].ModelName
		a.systemInfo.CpuModel = modelName
		// Extract short name before '@'
		if idx := strings.Index(modelName, "@"); idx > 0 {
			a.systemInfo.CpuModelShort = strings.TrimSpace(modelName[:idx])
		} else {
			a.systemInfo.CpuModelShort = modelName
		}
		// Set speed in GHz
		a.systemInfo.CpuSpeedGHz = fmt.Sprintf("%.2f GHz", info[0].Mhz/1000)
		// Set architecture
		a.systemInfo.CpuArch = runtime.GOARCH
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

	// Collect disk info (model/vendor)
	a.systemInfo.Disks = getDiskInfo()

	// Collect network interface info
	a.systemInfo.Networks = getNetworkInfo()

	// Collect memory module info
	a.systemInfo.Memory = getMemoryInfo()
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
	a.systemInfo.MemPct = systemStats.MemPct
	a.systemInfo.DiskPct = systemStats.DiskPct
	a.systemInfo.Uptime, _ = host.Uptime()
	a.systemInfo.Bandwidth = twoDecimals(systemStats.NetworkSent + systemStats.NetworkRecv)
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

func getMemoryInfo() []system.MemoryInfo {
	memInfo, err := ghwmem.New()
	if err != nil {
		slog.Debug("Failed to get memory info with ghw", "err", err)
		return nil
	}

	var memory []system.MemoryInfo
	for _, module := range memInfo.Modules {
		if module.Vendor == "" {
			continue
		}
		size := ""
		if module.SizeBytes > 0 {
			size = fmt.Sprintf("%d GB", module.SizeBytes/(1024*1024*1024))
		}
		memory = append(memory, system.MemoryInfo{
			Vendor: module.Vendor,
			Size:   size,
		})
	}
	return memory
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
