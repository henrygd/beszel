// Package btrfs reads btrfs filesystem state from sysfs.
package btrfs

// Filesystem is a mounted btrfs filesystem read from /sys/fs/btrfs/<uuid>.
type Filesystem struct {
	Name    string // label, or UUID when unlabeled
	Size    uint64 // total capacity of member devices in bytes
	Alloc   uint64 // raw bytes allocated to data, metadata and system chunks
	Health  string // ONLINE, or DEGRADED when a device is missing
	NRead   uint64 // cumulative bytes read across member devices
	NWrite  uint64 // cumulative bytes written across member devices
	Devices []Device
}

// Device is one member device (devinfo/<devid>) with its error counters.
type Device struct {
	Name           string // "devid N"; sysfs does not expose the block device path
	State          string // ONLINE or MISSING
	ReadErrs       uint64
	WriteErrs      uint64
	CorruptionErrs uint64
}
