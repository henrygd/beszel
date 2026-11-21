//go:build !linux

package lvm

import "errors"

func Pools() ([]Pool, error) {
	return nil, errors.ErrUnsupported
}

func IOCounters() ([]Counters, error) {
	return nil, errors.ErrUnsupported
}
