//go:build linux

package agent

import (
	"path/filepath"
	"strconv"

	"github.com/henrygd/beszel/agent/utils"
)

// containerMemoryMetrics is only used when USE_CGROUP is explicitly enabled.
// Unlike /proc/stat, lxcfs already serves a container-scoped /proc/meminfo,
// so LXC keeps the existing memory path by default.
func containerMemoryMetrics(hostTotal uint64, forceUseCgroup bool) (memoryMetrics, bool) {
	if !forceUseCgroup {
		return memoryMetrics{}, false
	}
	if utils.InCgroupV2(cpuProcSelfCgroup) {
		dir := cpuCgroupRoot
		if mount := utils.CgroupMountPoint(cpuCgroupMountinfo, "cgroup2", ""); mount != "" {
			dir = mount
		}
		if metrics, ok := readCgroupMemoryMetrics(dir, hostTotal, true); ok {
			return metrics, true
		}
	}
	if dir := utils.CgroupMountPoint(cpuCgroupMountinfo, "cgroup", "memory"); dir != "" {
		return readCgroupMemoryMetrics(dir, hostTotal, false)
	}
	return memoryMetrics{}, false
}

// Read the hierarchy mount root to include sibling services and descendants,
// rather than the agent's own service cgroup. A v1 unlimited limit is a
// page-aligned LONG_MAX sentinel; the threshold also covers 32-bit kernels.
func readCgroupMemoryMetrics(dir string, hostTotal uint64, v2 bool) (memoryMetrics, bool) {
	usageFile, limitFile, cacheKey, shmemKey := "memory.usage_in_bytes", "memory.limit_in_bytes", "total_cache", "total_shmem"
	if v2 {
		usageFile, limitFile, cacheKey, shmemKey = "memory.current", "memory.max", "file", "shmem"
	}
	usage, ok := utils.ReadUintFile(filepath.Join(dir, usageFile))
	if !ok {
		return memoryMetrics{}, false
	}
	rawLimit, ok := utils.ReadStringFileOK(filepath.Join(dir, limitFile))
	if !ok {
		return memoryMetrics{}, false
	}
	var limit uint64
	if !(v2 && rawLimit == "max") {
		var err error
		limit, err = strconv.ParseUint(rawLimit, 10, 64)
		if err != nil {
			return memoryMetrics{}, false
		}
		if !v2 && (limit >= (uint64(1)<<63)-(1<<20) || limit == 2147479552) {
			limit = 0 // v1 unlimited sentinel
		} else if limit == 0 {
			return memoryMetrics{}, false
		}
	}
	total := limit
	if hostTotal > 0 && (total == 0 || hostTotal < total) {
		total = hostTotal
	}
	if total == 0 {
		return memoryMetrics{}, false // unlimited and no usable host total
	}
	stat, err := utils.ReadCgroupStat(filepath.Join(dir, "memory.stat"))
	if err != nil {
		return memoryMetrics{}, false
	}
	cache, ok := stat[cacheKey]
	if !ok {
		return memoryMetrics{}, false
	}
	shmem, ok := stat[shmemKey]
	if !ok {
		return memoryMetrics{}, false
	}
	// Both cache (v1) and file (v2) include shmem/tmpfs, which must remain
	// charged as used memory rather than being classified as file cache.
	cache = min(saturatingSub(cache, shmem), usage)
	return memoryMetrics{Total: total, Used: usage - cache, BuffCache: cache}, true
}
