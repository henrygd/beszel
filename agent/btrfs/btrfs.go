// Package btrfs reads btrfs filesystem state from sysfs.
package btrfs

// Filesystem is a mounted btrfs filesystem read from /sys/fs/btrfs/<uuid>.
type Filesystem struct {
	UUID     string // stable filesystem UUID from sysfs
	MountID  string // kernel filesystem identity for matching monitored mounts
	IODevice string // sole member block-device name, empty for multi-device/unknown pools
	Name     string // label, else first mountpoint, else UUID
	Size     uint64 // effective usable capacity, or raw member capacity when Raw
	Raw      bool   // capacity and usage are physical bytes, unsuitable for disk alerts
	Alloc    uint64 // raw bytes allocated to data, metadata and system chunks
	Health   string // ONLINE, or DEGRADED when a device is missing
	NRead    uint64 // cumulative bytes read across member devices
	NWrite   uint64 // cumulative bytes written across member devices
	Devices  []Device
}

// Device is one member device (devinfo/<devid>) with its error counters.
type Device struct {
	Name           string // "devid N"; sysfs does not expose the block device path
	State          string // ONLINE or MISSING
	ReadErrs       uint64
	WriteErrs      uint64
	CorruptionErrs uint64
}
