// Package zfs defines the ZFS detail data exchanged between agent and hub.
package zfs

import "strings"

// ZfsData is the detail payload returned by the agent for the GetZfsData action.
type ZfsData struct {
	Pools    []*PoolDetail `json:"pools,omitempty"`
	Complete bool          `json:"complete,omitempty"`
	// Backends whose inventories are complete, even when another backend failed.
	CompleteBackends []string `json:"completeBackends,omitempty"`
}

// CanRefreshPool also governs deletion: missing pools may only be removed
// after a successful inventory of their backend. Complete supports old agents.
func (data *ZfsData) CanRefreshPool(name string) bool {
	if data.Complete {
		return true
	}
	backend := PoolBackend(name)
	for _, complete := range data.CompleteBackends {
		if complete == backend {
			return true
		}
	}
	return false
}

// AnyBackendComplete reports whether at least one backend inventory succeeded.
func (data *ZfsData) AnyBackendComplete() bool {
	return data.Complete || len(data.CompleteBackends) > 0
}

// PoolBackend returns the agent backend that reports a pool, from the prefix
// of its stable name.
func PoolBackend(name string) string {
	switch {
	case strings.HasPrefix(name, "b:"):
		return "btrfs"
	case strings.HasPrefix(name, "l:"):
		return "lvm"
	default:
		return "zfs"
	}
}

// PoolDetail holds the verbose state of a single pool: capacity, health,
// scrub, vdev, and dataset information.
type PoolDetail struct {
	DisplayName string     `json:"displayName,omitempty"`
	Raw         bool       `json:"raw,omitempty"`
	Name        string     `json:"name"`
	Health      string     `json:"health,omitempty"`
	Size        uint64     `json:"size,omitempty"`  // bytes
	Alloc       uint64     `json:"alloc,omitempty"` // bytes
	Free        uint64     `json:"free,omitempty"`  // bytes
	Scrub       *Scrub     `json:"scrub,omitempty"`
	Vdevs       []*Vdev    `json:"vdevs,omitempty"`
	Datasets    []*Dataset `json:"datasets,omitempty"`
}

// Scrub holds the scrub (or resilver) status of a pool.
type Scrub struct {
	State    string `json:"state,omitempty"` // NONE, SCANNING, FINISHED, CANCELED
	Progress string `json:"progress,omitempty"`
	Errors   uint64 `json:"errors,omitempty"`
}

// Vdev is a single vdev (mirror, raidz, or leaf disk) with error counters.
type Vdev struct {
	Name         string `json:"name"`
	State        string `json:"state,omitempty"`
	ReadErrs     uint64 `json:"readErrs,omitempty"`
	WriteErrs    uint64 `json:"writeErrs,omitempty"`
	ChecksumErrs uint64 `json:"checksumErrs,omitempty"`
}

// Dataset is a single ZFS dataset with usage information.
type Dataset struct {
	Name       string `json:"name"`
	Used       uint64 `json:"used,omitempty"`
	Avail      uint64 `json:"avail,omitempty"`
	Mountpoint string `json:"mount,omitempty"`
}
