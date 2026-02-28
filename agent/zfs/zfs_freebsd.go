//go:build freebsd

package zfs

import (
	"golang.org/x/sys/unix"
)

func ARCSize() (uint64, error) {
	return unix.SysctlUint64("kstat.zfs.misc.arcstats.size")
}
