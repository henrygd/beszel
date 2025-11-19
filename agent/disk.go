package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/shirou/gopsutil/v4/disk"
)

// parseFilesystemEntry parses a filesystem entry in the format "device__customname"
// Returns the device/filesystem part and the custom name part
func parseFilesystemEntry(entry string) (device, customName string) {
	entry = strings.TrimSpace(entry)
	if parts := strings.SplitN(entry, "__", 2); len(parts) == 2 {
		device = strings.TrimSpace(parts[0])
		customName = strings.TrimSpace(parts[1])
	} else {
		device = entry
	}
	return device, customName
}

// Sets up the filesystems to monitor for disk usage and I/O.
func (a *Agent) initializeDiskInfo() {
	filesystem, _ := GetEnv("FILESYSTEM")
	efPath := "/extra-filesystems"
	hasRoot := false
	isWindows := runtime.GOOS == "windows"

	partitions, err := disk.Partitions(false)
	if err != nil {
		slog.Error("Error getting disk partitions", "err", err)
	}
	slog.Debug("Disk", "partitions", partitions)

	// trim trailing backslash for Windows devices (#1361)
	if isWindows {
		for i, p := range partitions {
			partitions[i].Device = strings.TrimSuffix(p.Device, "\\")
		}
	}

	// ioContext := context.WithValue(a.sensorsContext,
	// 	common.EnvKey, common.EnvMap{common.HostProcEnvKey: "/tmp/testproc"},
	// )
	// diskIoCounters, err := disk.IOCountersWithContext(ioContext)

	diskIoCounters, err := disk.IOCounters()
	if err != nil {
		slog.Error("Error getting diskstats", "err", err)
	}
	slog.Debug("Disk I/O", "diskstats", diskIoCounters)

	// Helper function to add a filesystem to fsStats if it doesn't exist
	addFsStat := func(device, mountpoint string, root bool, customName ...string) {
		var key string
		if isWindows {
			key = device
		} else {
			key = filepath.Base(device)
		}
		var ioMatch bool
		if _, exists := a.fsStats[key]; !exists {
			if root {
				slog.Info("Detected root device", "name", key)
				// Check if root device is in /proc/diskstats, use fallback if not
				if _, ioMatch = diskIoCounters[key]; !ioMatch {
					key, ioMatch = findIoDevice(filesystem, diskIoCounters, a.fsStats)
					if !ioMatch {
						slog.Info("Using I/O fallback", "device", device, "mountpoint", mountpoint, "fallback", key)
					}
				}
			} else {
				// Check if non-root has diskstats and fall back to folder name if not
				// Scenario: device is encrypted and named luks-2bcb02be-999d-4417-8d18-5c61e660fb6e - not in /proc/diskstats.
				// However, the device can be specified by mounting folder from luks device at /extra-filesystems/sda1
				if _, ioMatch = diskIoCounters[key]; !ioMatch {
					efBase := filepath.Base(mountpoint)
					if _, ioMatch = diskIoCounters[efBase]; ioMatch {
						key = efBase
					}
				}
			}
			fsStats := &system.FsStats{Root: root, Mountpoint: mountpoint}
			if len(customName) > 0 && customName[0] != "" {
				fsStats.Name = customName[0]
			}
			a.fsStats[key] = fsStats
		}
	}

	// Get the appropriate root mount point for this system
	rootMountPoint := a.getRootMountPoint()

	// Use FILESYSTEM env var to find root filesystem
	if filesystem != "" {
		for _, p := range partitions {
			if strings.HasSuffix(p.Device, filesystem) || p.Mountpoint == filesystem {
				addFsStat(p.Device, p.Mountpoint, true)
				hasRoot = true
				break
			}
		}
		if !hasRoot {
			slog.Warn("Partition details not found", "filesystem", filesystem)
		}
	}

	// Add EXTRA_FILESYSTEMS env var values to fsStats
	if extraFilesystems, exists := GetEnv("EXTRA_FILESYSTEMS"); exists {
		for _, fsEntry := range strings.Split(extraFilesystems, ",") {
			// Parse custom name from format: device__customname
			fs, customName := parseFilesystemEntry(fsEntry)

			found := false
			for _, p := range partitions {
				if strings.HasSuffix(p.Device, fs) || p.Mountpoint == fs {
					addFsStat(p.Device, p.Mountpoint, false, customName)
					found = true
					break
				}
			}
			// if not in partitions, test if we can get disk usage
			if !found {
				if _, err := disk.Usage(fs); err == nil {
					addFsStat(filepath.Base(fs), fs, false, customName)
				} else {
					slog.Error("Invalid filesystem", "name", fs, "err", err)
				}
			}
		}
	}

	// Process partitions for various mount points
	for _, p := range partitions {
		// fmt.Println(p.Device, p.Mountpoint)
		// Binary root fallback or docker root fallback
		if !hasRoot && (p.Mountpoint == rootMountPoint || (p.Mountpoint == "/etc/hosts" && strings.HasPrefix(p.Device, "/dev"))) {
			fs, match := findIoDevice(filepath.Base(p.Device), diskIoCounters, a.fsStats)
			if match {
				addFsStat(fs, p.Mountpoint, true)
				hasRoot = true
			}
		}

		// Check if device is in /extra-filesystems
		if strings.HasPrefix(p.Mountpoint, efPath) {
			device, customName := parseFilesystemEntry(p.Mountpoint)
			addFsStat(device, p.Mountpoint, false, customName)
		}
	}

	// Check all folders in /extra-filesystems and add them if not already present
	if folders, err := os.ReadDir(efPath); err == nil {
		existingMountpoints := make(map[string]bool)
		for _, stats := range a.fsStats {
			existingMountpoints[stats.Mountpoint] = true
		}
		for _, folder := range folders {
			if folder.IsDir() {
				mountpoint := filepath.Join(efPath, folder.Name())
				slog.Debug("/extra-filesystems", "mountpoint", mountpoint)
				if !existingMountpoints[mountpoint] {
					device, customName := parseFilesystemEntry(folder.Name())
					addFsStat(device, mountpoint, false, customName)
				}
			}
		}
	}

	// If no root filesystem set, use fallback
	if !hasRoot {
		rootDevice, _ := findIoDevice(filepath.Base(filesystem), diskIoCounters, a.fsStats)
		slog.Info("Root disk", "mountpoint", rootMountPoint, "io", rootDevice)
		a.fsStats[rootDevice] = &system.FsStats{Root: true, Mountpoint: rootMountPoint}
	}

	a.initializeDiskIoStats(diskIoCounters)
}

// Returns matching device from /proc/diskstats,
// or the device with the most reads if no match is found.
// bool is true if a match was found.
func findIoDevice(filesystem string, diskIoCounters map[string]disk.IOCountersStat, fsStats map[string]*system.FsStats) (string, bool) {
	var maxReadBytes uint64
	maxReadDevice := "/"
	for _, d := range diskIoCounters {
		if d.Name == filesystem || (d.Label != "" && d.Label == filesystem) {
			return d.Name, true
		}
		if d.ReadBytes > maxReadBytes {
			// don't use if device already exists in fsStats
			if _, exists := fsStats[d.Name]; !exists {
				maxReadBytes = d.ReadBytes
				maxReadDevice = d.Name
			}
		}
	}
	return maxReadDevice, false
}

// Sets start values for disk I/O stats.
func (a *Agent) initializeDiskIoStats(diskIoCounters map[string]disk.IOCountersStat) {
	for device, stats := range a.fsStats {
		// skip if not in diskIoCounters
		d, exists := diskIoCounters[device]
		if !exists {
			slog.Warn("Device not found in diskstats", "name", device)
			continue
		}
		// populate initial values
		stats.Time = time.Now()
		stats.TotalRead = d.ReadBytes
		stats.TotalWrite = d.WriteBytes
		// add to list of valid io device names
		a.fsNames = append(a.fsNames, device)
	}
}

// Updates disk usage statistics for all monitored filesystems
func (a *Agent) updateDiskUsage(systemStats *system.Stats) {
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
}

// Updates disk I/O statistics for all monitored filesystems
func (a *Agent) updateDiskIo(cacheTimeMs uint16, systemStats *system.Stats) {
	// disk i/o (cache-aware per interval)
	if ioCounters, err := disk.IOCounters(a.fsNames...); err == nil {
		// Ensure map for this interval exists
		if _, ok := a.diskPrev[cacheTimeMs]; !ok {
			a.diskPrev[cacheTimeMs] = make(map[string]prevDisk)
		}
		now := time.Now()
		for name, d := range ioCounters {
			stats := a.fsStats[d.Name]
			if stats == nil {
				// skip devices not tracked
				continue
			}

			// Previous snapshot for this interval and device
			prev, hasPrev := a.diskPrev[cacheTimeMs][name]
			if !hasPrev {
				// Seed from agent-level fsStats if present, else seed from current
				prev = prevDisk{readBytes: stats.TotalRead, writeBytes: stats.TotalWrite, at: stats.Time}
				if prev.at.IsZero() {
					prev = prevDisk{readBytes: d.ReadBytes, writeBytes: d.WriteBytes, at: now}
				}
			}

			msElapsed := uint64(now.Sub(prev.at).Milliseconds())
			if msElapsed < 100 {
				// Avoid division by zero or clock issues; update snapshot and continue
				a.diskPrev[cacheTimeMs][name] = prevDisk{readBytes: d.ReadBytes, writeBytes: d.WriteBytes, at: now}
				continue
			}

			diskIORead := (d.ReadBytes - prev.readBytes) * 1000 / msElapsed
			diskIOWrite := (d.WriteBytes - prev.writeBytes) * 1000 / msElapsed
			readMbPerSecond := bytesToMegabytes(float64(diskIORead))
			writeMbPerSecond := bytesToMegabytes(float64(diskIOWrite))

			// validate values
			if readMbPerSecond > 50_000 || writeMbPerSecond > 50_000 {
				slog.Warn("Invalid disk I/O. Resetting.", "name", d.Name, "read", readMbPerSecond, "write", writeMbPerSecond)
				// Reset interval snapshot and seed from current
				a.diskPrev[cacheTimeMs][name] = prevDisk{readBytes: d.ReadBytes, writeBytes: d.WriteBytes, at: now}
				// also refresh agent baseline to avoid future negatives
				a.initializeDiskIoStats(ioCounters)
				continue
			}

			// Update per-interval snapshot
			a.diskPrev[cacheTimeMs][name] = prevDisk{readBytes: d.ReadBytes, writeBytes: d.WriteBytes, at: now}

			// Update global fsStats baseline for cross-interval correctness
			stats.Time = now
			stats.TotalRead = d.ReadBytes
			stats.TotalWrite = d.WriteBytes
			stats.DiskReadPs = readMbPerSecond
			stats.DiskWritePs = writeMbPerSecond
			stats.DiskReadBytes = diskIORead
			stats.DiskWriteBytes = diskIOWrite

			if stats.Root {
				systemStats.DiskReadPs = stats.DiskReadPs
				systemStats.DiskWritePs = stats.DiskWritePs
				systemStats.DiskIO[0] = diskIORead
				systemStats.DiskIO[1] = diskIOWrite
			}
		}
	}
}

// getRootMountPoint returns the appropriate root mount point for the system
// For immutable systems like Fedora Silverblue, it returns /sysroot instead of /
func (a *Agent) getRootMountPoint() string {
	// 1. Check if /etc/os-release contains indicators of an immutable system
	if osReleaseContent, err := os.ReadFile("/etc/os-release"); err == nil {
		content := string(osReleaseContent)
		if strings.Contains(content, "fedora") && strings.Contains(content, "silverblue") ||
			strings.Contains(content, "coreos") ||
			strings.Contains(content, "flatcar") ||
			strings.Contains(content, "rhel-atomic") ||
			strings.Contains(content, "centos-atomic") {
			// Verify that /sysroot exists before returning it
			if _, err := os.Stat("/sysroot"); err == nil {
				return "/sysroot"
			}
		}
	}

	// 2. Check if /run/ostree is present (ostree-based systems like Silverblue)
	if _, err := os.Stat("/run/ostree"); err == nil {
		// Verify that /sysroot exists before returning it
		if _, err := os.Stat("/sysroot"); err == nil {
			return "/sysroot"
		}
	}

	return "/"
}
