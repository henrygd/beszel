//go:build linux

package btrfs

import (
	"bytes"
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
	deviceInfo      = ioctlDeviceInfo
	filesystemInfo  = ioctlFilesystemInfo
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
	// devices/<name> links to the block device's sysfs directory. Only
	// present devices appear, so the count doubles as a health check.
	devices, err := os.ReadDir(filepath.Join(dir, "devices"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fs, err
	}
	attached := -1
	if err == nil {
		attached = len(devices)
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
	// Kernels without sysfs devinfo (pre-5.13, e.g. Synology DSM) expose the
	// same device state through ioctls on the mountpoint.
	if errors.Is(err, os.ErrNotExist) {
		capacityAvailable, healthKnown = fs.readDevicesIoctl(mountpoint, attached)
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
	if len(devices) == 1 && len(fs.Devices) == 1 && fs.Health == "ONLINE" {
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

// readDevicesIoctl determines member devices, capacity and health through the
// BTRFS_IOC_FS_INFO and BTRFS_IOC_DEV_INFO ioctls when sysfs devinfo is
// unavailable. It returns (capacityAvailable, healthKnown) for the shared
// accounting in readFilesystem, and stays inert when the mountpoint cannot
// be queried. attached is the sysfs devices count, or -1 when unknown.
func (fs *Filesystem) readDevicesIoctl(mountpoint string, attached int) (capacityAvailable, healthKnown bool) {
	info, err := filesystemInfo(mountpoint)
	if err != nil || info.numDevices == 0 {
		return false, false
	}
	var members []Device
	var size, probed, missing uint64
	for devid := uint64(1); devid <= info.maxID && probed < info.numDevices; devid++ {
		dev, err := deviceInfo(mountpoint, devid)
		if errors.Is(err, unix.ENODEV) {
			continue // the devid was freed by a device remove/replace
		}
		if err != nil {
			return false, false
		}
		probed++
		size += dev.totalBytes
		member := Device{Name: "devid " + strconv.FormatUint(devid, 10), State: "ONLINE"}
		if dev.missing {
			member.State = "MISSING"
			missing++
			fs.Health = "DEGRADED"
		}
		members = append(members, member)
	}
	if probed != info.numDevices {
		return false, false // a device appeared or vanished mid-probe
	}
	// The kernel counts missing members too, so a number above the sysfs
	// devices count means one is missing even if no path proved it. Members
	// the missing one hides among can no longer be verified.
	if attached >= 0 && uint64(attached)+missing < info.numDevices {
		fs.Health = "DEGRADED"
		for i := range members {
			if members[i].State == "ONLINE" {
				members[i].State = "UNKNOWN"
			}
		}
	}
	fs.Devices = members
	fs.Size += size
	return true, true
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

// deviceInfoArgs is the subset of struct btrfs_ioctl_dev_info_args the agent
// uses: the recorded capacity and whether the member is missing.
type deviceInfoArgs struct {
	totalBytes uint64
	missing    bool
}

// ioctlDeviceInfo reads a member device's state with BTRFS_IOC_DEV_INFO,
// _IOWR(0x94, 30, struct btrfs_ioctl_dev_info_args), a 4096-byte ABI
// structure. Missing members answer the ioctl but report no device path
// (dev->name is NULL, or the "<missing disk>" placeholder on kernels that
// print via btrfs_dev_name); freed devids fail with ENODEV.
func ioctlDeviceInfo(mountpoint string, devid uint64) (deviceInfoArgs, error) {
	var info deviceInfoArgs
	if mountpoint == "" {
		return info, errors.New("no accessible mountpoint")
	}
	f, err := os.Open(mountpoint)
	if err != nil {
		return info, err
	}
	defer f.Close()
	args := struct {
		Devid      uint64
		UUID       [16]byte
		BytesUsed  uint64
		TotalBytes uint64
		Reserved   [3072 - 40]byte
		Path       [1024]byte
	}{Devid: devid}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), 0xd000941e, uintptr(unsafe.Pointer(&args)))
	if errno != 0 {
		return info, errno
	}
	info.totalBytes = args.TotalBytes
	name := args.Path[:]
	if end := bytes.IndexByte(name, 0); end >= 0 {
		name = name[:end]
	}
	info.missing = len(name) == 0 || strings.HasPrefix(string(name), "<missing")
	return info, nil
}

// ioctlDeviceSize reads Btrfs's recorded device size, which can be smaller
// than the block device after a filesystem resize.
func ioctlDeviceSize(mountpoint string, devid uint64) (uint64, error) {
	info, err := ioctlDeviceInfo(mountpoint, devid)
	return info.totalBytes, err
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

// fsInfoArgs is the subset of struct btrfs_ioctl_fs_info_args the agent
// uses: the highest devid, the member count including missing devices, and
// the filesystem UUID.
type fsInfoArgs struct {
	maxID      uint64
	numDevices uint64
	fsid       [16]byte
}

// ioctlFilesystemInfo reads BTRFS_IOC_FS_INFO for a mounted btrfs path.
func ioctlFilesystemInfo(path string) (fsInfoArgs, error) {
	var info fsInfoArgs
	var stat unix.Statfs_t
	if unix.Statfs(path, &stat) != nil || !isBtrfs(&stat) {
		return info, errors.New("mountpoint is not Btrfs")
	}
	f, err := os.Open(path)
	if err != nil {
		return info, err
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
		return info, errno
	}
	info.maxID, info.numDevices, info.fsid = args.MaxID, args.NumDevices, args.FSID
	return info, nil
}

// MountID returns the filesystem UUID via BTRFS_IOC_FS_INFO. Unlike statfs
// f_fsid, this identity is shared by all subvolumes and bind mounts.
func MountID(path string) string {
	if path == "" {
		return ""
	}
	info, err := ioctlFilesystemInfo(path)
	if err != nil {
		return ""
	}
	id := info.fsid
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
