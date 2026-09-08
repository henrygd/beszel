//go:build !linux

package btrfs

import "errors"

func Filesystems() ([]Filesystem, error) {
	return nil, errors.ErrUnsupported
}
