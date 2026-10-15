//go:build !linux

package zfs

// The /dev/zfs probe is Linux-specific. Other platforms detect availability
// through the ZFS utilities themselves.
func checkZfsDevice() error {
	return nil
}
