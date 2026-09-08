//go:build linux

package btrfs

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	sysfsPath  = "/sys/fs/btrfs"
	mountsPath = "/proc/self/mounts"
)

// Filesystems returns all mounted btrfs filesystems, or nil when there are none.
func Filesystems() ([]Filesystem, error) {
	entries, err := os.ReadDir(sysfsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	mounts := mountpointsByDevice()
	var filesystems []Filesystem
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "features" {
			continue
		}
		filesystems = append(filesystems, readFilesystem(filepath.Join(sysfsPath, entry.Name()), mounts))
	}
	return filesystems, nil
}

func readFilesystem(dir string, mounts map[string]string) Filesystem {
	fs := Filesystem{Name: readString(filepath.Join(dir, "label")), Health: "ONLINE"}
	for _, kind := range []string{"data", "metadata", "system"} {
		fs.Alloc += readUint(filepath.Join(dir, "allocation", kind, "disk_used"))
	}
	// devices/<name> links to the block device's sysfs directory.
	devices, _ := os.ReadDir(filepath.Join(dir, "devices"))
	for _, dev := range devices {
		if fs.Name == "" {
			fs.Name = mounts[dev.Name()]
		}
		devDir := filepath.Join(dir, "devices", dev.Name())
		fs.Size += readUint(filepath.Join(devDir, "size")) * 512
		if stat := strings.Fields(readString(filepath.Join(devDir, "stat"))); len(stat) >= 7 {
			fs.NRead += parseUint(stat[2]) * 512
			fs.NWrite += parseUint(stat[6]) * 512
		}
	}
	devids, _ := os.ReadDir(filepath.Join(dir, "devinfo"))
	for _, devid := range devids {
		devDir := filepath.Join(dir, "devinfo", devid.Name())
		dev := Device{Name: "devid " + devid.Name(), State: "ONLINE"}
		if readUint(filepath.Join(devDir, "missing")) == 1 {
			dev.State = "MISSING"
			fs.Health = "DEGRADED"
		}
		for line := range strings.Lines(readString(filepath.Join(devDir, "error_stats"))) {
			if fields := strings.Fields(line); len(fields) == 2 {
				switch fields[0] {
				case "read_errs":
					dev.ReadErrs = parseUint(fields[1])
				case "write_errs":
					dev.WriteErrs = parseUint(fields[1])
				case "corruption_errs":
					dev.CorruptionErrs = parseUint(fields[1])
				}
			}
		}
		fs.Devices = append(fs.Devices, dev)
	}
	if fs.Name == "" {
		fs.Name = filepath.Base(dir)
	}
	return fs
}

// mountpointsByDevice maps each btrfs mount's source device name (as it
// appears under sysfs devices/, e.g. sda1 or dm-0) to its first mountpoint.
func mountpointsByDevice() map[string]string {
	mounts := make(map[string]string)
	for line := range strings.Lines(readString(mountsPath)) {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[2] != "btrfs" {
			continue
		}
		device := fields[0]
		if resolved, err := filepath.EvalSymlinks(device); err == nil {
			device = resolved
		}
		if _, seen := mounts[filepath.Base(device)]; !seen {
			mounts[filepath.Base(device)] = fields[1]
		}
	}
	return mounts
}

// Missing or unreadable sysfs attributes (older kernels) are treated as empty.
func readString(path string) string {
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}

func readUint(path string) uint64 {
	return parseUint(readString(path))
}

func parseUint(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}
