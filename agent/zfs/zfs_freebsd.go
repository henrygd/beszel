//go:build freebsd

package zfs

import (
	"errors"

	"golang.org/x/sys/unix"
)

func ARCSize() (uint64, error) {
	return unix.SysctlUint64("kstat.zfs.misc.arcstats.size")
}

// FreeBSD does not expose Linux's per-pool procfs kstats. Capacity, health,
// and detail collection still work through the cached utilities.
func PoolKernelStats() ([]PoolKernelStat, error) {
	return nil, errors.ErrUnsupported
}
