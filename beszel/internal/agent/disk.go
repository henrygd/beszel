package agent

import (
	"beszel/internal/entities/system"
	"log/slog"
	"time"

	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
)

// Sets up the filesystems to monitor for disk usage and I/O.
func (a *Agent) initializeDiskInfo() {
	filesystem := os.Getenv("FILESYSTEM")
	efPath := "/extra-filesystems"
	hasRoot := false

	partitions, err := disk.Partitions(false)
	if err != nil {
		slog.Error("Error getting disk partitions", "err", err)
	}
	slog.Debug("Disk", "partitions", partitions)

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
	addFsStat := func(device, mountpoint string, root bool) {
		key := filepath.Base(device)
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
			a.fsStats[key] = &system.FsStats{Root: root, Mountpoint: mountpoint}
		}
	}

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
	if extraFilesystems, exists := os.LookupEnv("EXTRA_FILESYSTEMS"); exists {
		for _, fs := range strings.Split(extraFilesystems, ",") {
			found := false
			for _, p := range partitions {
				if strings.HasSuffix(p.Device, fs) || p.Mountpoint == fs {
					addFsStat(p.Device, p.Mountpoint, false)
					found = true
					break
				}
			}
			// if not in partitions, test if we can get disk usage
			if !found {
				if _, err := disk.Usage(fs); err == nil {
					addFsStat(filepath.Base(fs), fs, false)
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
		if !hasRoot && (p.Mountpoint == "/" || (p.Mountpoint == "/etc/hosts" && strings.HasPrefix(p.Device, "/dev"))) {
			fs, match := findIoDevice(filepath.Base(p.Device), diskIoCounters, a.fsStats)
			if match {
				addFsStat(fs, p.Mountpoint, true)
				hasRoot = true
			}
		}

		// Check if device is in /extra-filesystems
		if strings.HasPrefix(p.Mountpoint, efPath) {
			addFsStat(p.Device, p.Mountpoint, false)
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
					addFsStat(folder.Name(), mountpoint, false)
				}
			}
		}
	}

	// If no root filesystem set, use fallback
	if !hasRoot {
		rootDevice, _ := findIoDevice(filepath.Base(filesystem), diskIoCounters, a.fsStats)
		slog.Info("Root disk", "mountpoint", "/", "io", rootDevice)
		a.fsStats[rootDevice] = &system.FsStats{Root: true, Mountpoint: "/"}
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
