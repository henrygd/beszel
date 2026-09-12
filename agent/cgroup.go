package agent

import (
	"os"
	"strconv"
	"strings"
)

// cgroupMemoryStats holds the memory limit and usage of the cgroup the agent
// runs in. It is the reliable source of container memory figures when the agent
// runs inside an LXC / Proxmox container (where /proc/meminfo still describes the
// physical host) or under a `docker run --memory` / Kubernetes limit.
type cgroupMemoryStats struct {
	limit   uint64 // hard memory limit in bytes
	used    uint64 // working set: current usage minus reclaimable page cache
	cache   uint64 // file-backed page cache in bytes
	limitOK bool   // a finite limit is configured
	usageOK bool   // per-cgroup usage accounting is exposed
}

// cgroup v1 reports "unlimited" as a page-aligned value close to the max int64.
// Anything at or above this is treated as no limit.
const cgroupV1Unlimited = uint64(1) << 62

// cgroup file locations, kept as vars so tests can point them at fixtures.
var (
	cgroupV2Root = "/sys/fs/cgroup"
	cgroupV1Root = "/sys/fs/cgroup/memory"
)

// readCgroupMemory reads the current cgroup's memory limit and usage, preferring
// the cgroup v2 unified hierarchy and falling back to cgroup v1. Every field is
// best-effort: callers must check limitOK / usageOK.
func readCgroupMemory() (s cgroupMemoryStats) {
	if readCgroupV2Memory(&s) {
		return s
	}
	readCgroupV1Memory(&s)
	return s
}

// readCgroupV2Memory populates s from the unified hierarchy. It returns true when
// the v2 memory controller files are present (even if no limit is set), so the
// caller knows not to try v1.
func readCgroupV2Memory(s *cgroupMemoryStats) bool {
	raw, err := os.ReadFile(cgroupV2Root + "/memory.max")
	if err != nil {
		return false
	}
	if limit, ok := parseUint(strings.TrimSpace(string(raw))); ok {
		s.limit, s.limitOK = limit, true
	}
	if current, ok := readUintFile(cgroupV2Root + "/memory.current"); ok {
		// memory.stat "file" is the total file-backed page cache charged to the
		// cgroup; subtracting it yields a working set comparable to how the hub
		// presents host "used" memory (buff/cache excluded).
		cache := readCgroupStatKey(cgroupV2Root+"/memory.stat", "file")
		s.cache = cache
		s.used = saturatingSub(current, cache)
		s.usageOK = true
	}
	return true
}

// readCgroupV1Memory populates s from the legacy hierarchy.
func readCgroupV1Memory(s *cgroupMemoryStats) {
	if limit, ok := readUintFile(cgroupV1Root + "/memory.limit_in_bytes"); ok {
		if limit > 0 && limit < cgroupV1Unlimited {
			s.limit, s.limitOK = limit, true
		}
	}
	if usage, ok := readUintFile(cgroupV1Root + "/memory.usage_in_bytes"); ok {
		cache := readCgroupStatKey(cgroupV1Root+"/memory.stat", "total_cache")
		if cache == 0 {
			cache = readCgroupStatKey(cgroupV1Root+"/memory.stat", "cache")
		}
		s.cache = cache
		s.used = saturatingSub(usage, cache)
		s.usageOK = true
	}
}

// readCgroupStatKey returns the value for a key in a cgroup "stat" file
// (space-separated "key value" lines), or 0 if the key or file is missing.
func readCgroupStatKey(path, key string) uint64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		name, value, ok := strings.Cut(line, " ")
		if !ok || name != key {
			continue
		}
		if v, ok := parseUint(strings.TrimSpace(value)); ok {
			return v
		}
		return 0
	}
	return 0
}

// readUintFile reads a file whose entire contents are a single unsigned integer.
func readUintFile(path string) (uint64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	return parseUint(strings.TrimSpace(string(raw)))
}

// parseUint parses a base-10 unsigned integer, rejecting cgroup sentinels like
// "max".
func parseUint(s string) (uint64, bool) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
