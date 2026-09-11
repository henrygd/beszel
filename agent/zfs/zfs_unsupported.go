//go:build !linux && !freebsd

package zfs

import "errors"

func ARCSize() (uint64, error) {
	return 0, errors.ErrUnsupported
}

func PoolStats() ([]PoolStat, error) {
	return nil, errors.ErrUnsupported
}

func PoolKernelStats() ([]PoolKernelStat, error) {
	return nil, errors.ErrUnsupported
}
