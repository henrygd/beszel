//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/henrygd/beszel/agent/btrfs"
	"github.com/henrygd/beszel/agent/utils"
	"golang.org/x/sys/unix"
)

// sysRoot is the sysfs mount; a variable so tests can fake the block device tree.
var sysRoot = "/sys"

const zfsSuperMagic = 0x2fc12fc1

// statMount describes the filesystem mounted at path. It reads only stat(2),
// statfs(2) and sysfs, which a default Docker container can access.
var statMount = func(path string) (m mountInfo) {
	var st unix.Stat_t
	if unix.Stat(path, &st) != nil {
		return m
	}
	major, minor := unix.Major(uint64(st.Dev)), unix.Minor(uint64(st.Dev))
	m.id = fmt.Sprintf("dev:%d:%d", major, minor)
	// Major 0 is an anonymous device (overlay, btrfs, ZFS, NFS, FUSE) with no
	// diskstats entry.
	if major != 0 {
		m.devName = blockDeviceName(major, minor)
	}

	var sfs unix.Statfs_t
	if unix.Statfs(path, &sfs) != nil {
		return m
	}
	switch uint32(sfs.Type) {
	case unix.OVERLAYFS_SUPER_MAGIC:
		m.overlay = true
	case unix.BTRFS_SUPER_MAGIC:
		// Each subvolume has its own anonymous st_dev, but they all share one
		// filesystem and therefore one usage figure.
		if id := btrfs.MountID(path); id != "" {
			m.id = "btrfs:" + id
			m.stableID = m.id
		}
	case unix.EXT4_SUPER_MAGIC, zfsSuperMagic:
		// ext2/3/4 derive f_fsid from the filesystem UUID and ZFS from the
		// dataset GUID. Most other filesystems use the device number, which
		// is exactly what changes between boots.
		if sfs.Fsid.Val[0] != 0 || sfs.Fsid.Val[1] != 0 {
			m.stableID = fmt.Sprintf("fsid:%08x%08x", uint32(sfs.Fsid.Val[0]), uint32(sfs.Fsid.Val[1]))
		}
	}
	if m.stableID == "" && m.devName != "" {
		m.stableID = blockDeviceID(m.devName)
	}
	return m
}

// blockDeviceName returns the kernel name of a block device, which is also
// its /proc/diskstats name (e.g. 259:2 -> nvme0n1p2, 253:0 -> dm-0).
func blockDeviceName(major, minor uint32) string {
	target, err := os.Readlink(filepath.Join(sysRoot, "dev", "block", fmt.Sprintf("%d:%d", major, minor)))
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// blockDeviceID returns a persistent identifier for a block device from
// sysfs: the partition UUID, the device-mapper or md UUID, or the disk WWID.
func blockDeviceID(name string) string {
	dir := filepath.Join(sysRoot, "class", "block", name)
	for line := range strings.SplitSeq(utils.ReadStringFile(filepath.Join(dir, "uevent")), "\n") {
		if uuid, ok := strings.CutPrefix(line, "PARTUUID="); ok && uuid != "" {
			return "part:" + uuid
		}
	}
	for _, f := range []struct{ prefix, path string }{
		{"dm:", "dm/uuid"},
		{"md:", "md/uuid"},
		{"wwid:", "wwid"},
		{"wwid:", "device/wwid"},
	} {
		if v := utils.ReadStringFile(filepath.Join(dir, f.path)); v != "" {
			return f.prefix + v
		}
	}
	return ""
}
