package agent

import (
	"beszel/internal/entities/system"
	"time"

	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
)

// problem: device is in partitions, but not in io counters
// solution: if filesystem exists, always use for io counters, even if root is

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
