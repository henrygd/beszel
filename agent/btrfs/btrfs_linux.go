//go:build linux

package btrfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"github.com/henrygd/beszel/agent/utils"
	"golang.org/x/sys/unix"
)

var (
	sysfsPath       = "/sys/fs/btrfs"
	mountsPath      = "/proc/self/mounts"
	mountinfoPath   = "/proc/self/mountinfo"
	mountUUID       = MountID
	deviceSize      = ioctlDeviceSize
	filesystemUsage = statfsUsage
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
		fs, err := readFilesystem(filepath.Join(sysfsPath, entry.Name()), mounts)
		if err != nil {
			return nil, fmt.Errorf("btrfs %s: %w", entry.Name(), err)
		}
		filesystems = append(filesystems, fs)
	}
	return filesystems, nil
}

func readFilesystem(dir string, mounts map[string]string) (Filesystem, error) {
	fs := Filesystem{UUID: filepath.Base(dir), Name: utils.ReadStringFile(filepath.Join(dir, "label")), Health: "UNKNOWN"}
	for _, kind := range []string{"data", "metadata", "system"} {
		if value, ok := utils.ReadUintFile(filepath.Join(dir, "allocation", kind, "disk_used")); ok {
			fs.Alloc += value
		}
	}
	// devices/<name> links to the block device's sysfs directory.
	devices, err := os.ReadDir(filepath.Join(dir, "devices"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fs, err
	}
	mountpoint := mounts["uuid:"+fs.UUID]
	if fs.Name == "" {
		fs.Name = mountpoint
	}
	var backingSize uint64
	for _, dev := range devices {
		if mountpoint == "" {
			mountpoint = mounts[dev.Name()]
		}
		if fs.Name == "" {
			fs.Name = mountpoint
		}
		devDir := filepath.Join(dir, "devices", dev.Name())
		if size, ok := utils.ReadUintFile(filepath.Join(devDir, "size")); ok {
			backingSize += size * 512
		}
		if stat := strings.Fields(utils.ReadStringFile(filepath.Join(devDir, "stat"))); len(stat) >= 7 {
			fs.NRead += parseUint(stat[2]) * 512
			fs.NWrite += parseUint(stat[6]) * 512
		}
	}
	devids, err := os.ReadDir(filepath.Join(dir, "devinfo"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fs, err
	}
	capacityAvailable := len(devids) > 0
	healthKnown := len(devids) > 0
	for _, devid := range devids {
		devDir := filepath.Join(dir, "devinfo", devid.Name())
		// Replacement targets do not add filesystem capacity.
		replaceTarget, _ := utils.ReadUintFile(filepath.Join(devDir, "replace_target"))
		if replaceTarget != 1 {
			devid, err := strconv.ParseUint(devid.Name(), 10, 64)
			if err != nil {
				return fs, err
			}
			size, err := deviceSize(mountpoint, devid)
			if err != nil {
				capacityAvailable = false
			}
			fs.Size += size
		}
		dev := Device{Name: "devid " + devid.Name(), State: "ONLINE"}
		missing := utils.ReadStringFile(filepath.Join(devDir, "missing"))
		if missing != "0" && missing != "1" {
			healthKnown = false
			dev.State = "UNKNOWN"
		}
		if missing == "1" {
			dev.State = "MISSING"
			fs.Health = "DEGRADED"
		}
		for line := range strings.Lines(utils.ReadStringFile(filepath.Join(devDir, "error_stats"))) {
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
	// Use one capacity source for the whole filesystem: device IDs cannot be
	// reliably matched to block-device names in sysfs. A partial ioctl result
	// must not be added to the complete backing-device total.
	if !capacityAvailable {
		fs.Size = backingSize
	}
	if fs.Health != "DEGRADED" && healthKnown {
		fs.Health = "ONLINE"
	}
	fs.MountID = mountUUID(mountpoint)
	if len(devices) == 1 && len(devids) == 1 && fs.Health == "ONLINE" {
		fs.IODevice = devices[0].Name()
	}
	fs.Raw = true
	if used, available, err := filesystemUsage(mountpoint); err == nil {
		// Effective capacity excludes reserved/unavailable space, so Size-Alloc
		// is available to applications and the usage ratio matches df.
		fs.Size, fs.Alloc, fs.Raw = used+available, used, false
	}
	if fs.Name == "" {
		fs.Name = filepath.Base(dir)
	}
	return fs, nil
}

// mountpointsByDevice prefers UUID matches from mountinfo and retains source
// device names as a fallback for environments where FS_INFO is unavailable.
func mountpointsByDevice() map[string]string {
	mounts := mountpointsByUUID(utils.ReadStringFile(mountinfoPath), mountUUID)
	for line := range strings.Lines(utils.ReadStringFile(mountsPath)) {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[2] != "btrfs" {
			continue
		}
		device := fields[0]
		if resolved, err := filepath.EvalSymlinks(device); err == nil {
			device = resolved
		}
		if _, seen := mounts[filepath.Base(device)]; !seen {
			mounts[filepath.Base(device)] = unescapeMountPath(fields[1])
		}
	}
	return mounts
}

func parseUint(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}

// ioctlDeviceSize reads Btrfs's recorded device size, which can be smaller
// than the block device after a filesystem resize. BTRFS_IOC_DEV_INFO is
// _IOWR(0x94, 30, struct btrfs_ioctl_dev_info_args), a 4096-byte ABI structure.
func ioctlDeviceSize(mountpoint string, devid uint64) (uint64, error) {
	if mountpoint == "" {
		return 0, errors.New("no accessible mountpoint")
	}
	f, err := os.Open(mountpoint)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	args := struct {
		Devid      uint64
		UUID       [16]byte
		BytesUsed  uint64
		TotalBytes uint64
		Reserved   [4096 - 40]byte
	}{Devid: devid}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), 0xd000941e, uintptr(unsafe.Pointer(&args)))
	if errno != 0 {
		return 0, errno
	}
	return args.TotalBytes, nil
}

// The filesystem magic is unsigned even when Statfs_t.Type is int32.
func isBtrfs(stat *unix.Statfs_t) bool {
	return uint32(stat.Type) == unix.BTRFS_SUPER_MAGIC
}

func statfsUsage(path string) (used, available uint64, err error) {
	if path == "" {
		return 0, 0, errors.New("no accessible mountpoint")
	}
	var stat unix.Statfs_t
	if err = unix.Statfs(path, &stat); err != nil {
		return
	}
	if !isBtrfs(&stat) {
		return 0, 0, errors.New("mountpoint is not Btrfs")
	}
	blockSize := uint64(stat.Bsize)
	return (stat.Blocks - min(stat.Blocks, stat.Bfree)) * blockSize, min(stat.Blocks, stat.Bavail) * blockSize, nil
}

// MountID returns the filesystem UUID via BTRFS_IOC_FS_INFO. Unlike statfs
// f_fsid, this identity is shared by all subvolumes and bind mounts.
func MountID(path string) string {
	if path == "" {
		return ""
	}
	var stat unix.Statfs_t
	if unix.Statfs(path, &stat) != nil || !isBtrfs(&stat) {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	args := struct {
		MaxID      uint64
		NumDevices uint64
		FSID       [16]byte
		Reserved   [992]byte
	}{}
	// _IOR(0x94, 31, 1024). Reuse the platform's read-direction bits;
	// MIPS/PowerPC use a different encoding than asm-generic.
	request := uintptr(unix.FS_IOC_GETFLAGS&0xe0000000) | 0x0400941f
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), request, uintptr(unsafe.Pointer(&args)))
	if errno != 0 {
		return ""
	}
	id := args.FSID
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}

// Btrfs mountinfo device numbers can be virtual (0:N), so query the UUID
// through the mount instead of comparing those numbers with sysfs block devs.
// Retry another path when a bind mount is inaccessible. Once resolved, reuse
// the result for that mount device to avoid opening every Docker bind mount.
func mountpointsByUUID(mountinfo string, identify func(string) string) map[string]string {
	mounts := make(map[string]string)
	resolved := make(map[string]bool)
	for line := range strings.Lines(mountinfo) {
		before, after, ok := strings.Cut(line, " - ")
		fields, fs := strings.Fields(before), strings.Fields(after)
		if !ok || len(fields) < 6 || len(fs) < 3 || fs[0] != "btrfs" || resolved[fields[2]] {
			continue
		}
		path := unescapeMountPath(fields[4])
		uuid := identify(path)
		if uuid == "" {
			continue
		}
		resolved[fields[2]] = true
		if mounts["uuid:"+uuid] == "" {
			mounts["uuid:"+uuid] = path
		}
	}
	return mounts
}

func unescapeMountPath(path string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(path)
}
