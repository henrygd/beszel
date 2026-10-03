//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
)

// LXC-aware CPU accounting (issue #2332).
//
// Inside an LXC guest, lxcfs serves /proc/stat with the raw counters of the
// host cores in the guest's cpuset, not the guest's own usage. An idle guest
// sharing a host core with a busy neighbor then reports near-100% CPU while
// doing nothing. The cgroup's own accounting (cpu.stat / cpuacct.usage)
// reflects only the guest's processes, so inside LXC we derive CPU% from that
// instead.
//
// Other runtimes (Docker, Podman, k8s) use /proc/stat by default: the agent
// is normally deployed there to monitor the host. BESZEL_AGENT_USE_CGROUP=true
// explicitly opts into cgroup-root CPU accounting outside LXC.

// File paths and hooks are variables so tests can point them at fixtures.
var (
	cpuCgroupRoot      = "/sys/fs/cgroup" // default cgroup v2 mount point
	cpuCgroupMountinfo = "/proc/self/mountinfo"
	cpuProcSelfCgroup  = "/proc/self/cgroup"
	cpuSystemdContPath = "/run/systemd/container"
	cpuNumCPU          = runtime.NumCPU
	cpuNow             = time.Now
)

// cpuUserHZ is the USER_HZ jiffies-per-second rate cpuacct.stat reports in.
const cpuUserHZ = 100

// inLxc reports whether the agent itself runs inside an LXC guest.
// The result is cached because it cannot change during the process lifetime.
var inLxc = sync.OnceValue(detectLxc)

// detectLxc looks for LXC guest markers that are readable without root
// (the agent usually runs as an unprivileged user, so /proc/1/environ is not).
func detectLxc() bool {
	// lxcfs mounted over /proc/stat is the direct cause of the host-core
	// counters. Only match that mount point: an LXC host also has lxcfs
	// mounted, but at /var/lib/lxcfs.
	if data, err := os.ReadFile(cpuCgroupMountinfo); err == nil && procStatFromLxcfs(data) {
		return true
	}
	// set by liblxc for the container init and inherited on non-systemd guests
	if os.Getenv("container") == "lxc" {
		return true
	}
	// written by systemd on systemd-based guests
	if data, err := os.ReadFile(cpuSystemdContPath); err == nil &&
		strings.TrimSpace(string(data)) == "lxc" {
		return true
	}
	return false
}

// useCgroupCpu enables cgroup-root CPU accounting automatically in LXC or
// explicitly when requested by the agent configuration.
func useCgroupCpu(forceUse bool) bool {
	return forceUse || inLxc()
}

// procStatFromLxcfs reports whether mountinfo shows lxcfs mounted on /proc/stat.
func procStatFromLxcfs(mountinfo []byte) bool {
	for line := range strings.SplitSeq(string(mountinfo), "\n") {
		left, right, found := strings.Cut(line, " - ")
		if !found {
			continue
		}
		fields, post := strings.Fields(left), strings.Fields(right)
		if len(fields) >= 5 && len(post) > 0 && fields[4] == "/proc/stat" && post[0] == "fuse.lxcfs" {
			return true
		}
	}
	return false
}

// cgroupCpuSample is one read of the container's cumulative CPU accounting.
type cgroupCpuSample struct {
	usageUsec  uint64
	userUsec   uint64
	systemUsec uint64
	cores      float64 // usable CPU cores: affinity ∩ cpuset ∩ quota
	at         time.Time
}

var lastCgroupCpuSamples = make(map[uint16]cgroupCpuSample)

// initializeCpu seeds the cgroup baseline after Agent configuration is read,
// so the first reported CPU value is a delta instead of zero.
func (a *Agent) initializeCpu() {
	if !useCgroupCpu(a.forceUseCgroup) {
		return
	}
	if s, ok := readContainerCpuSample(); ok {
		s.at = cpuNow()
		lastCgroupCpuSamples[60000] = s
	}
}

// containerCpuMetrics derives CPU metrics from the cgroup mount root's
// accounting when running inside LXC or explicitly enabled. It returns
// ok=false when disabled or whenever cgroup accounting is unreadable, so
// callers keep the /proc/stat path.
func containerCpuMetrics(cacheTimeMs uint16, forceUseCgroup bool) (CpuMetrics, bool) {
	if !useCgroupCpu(forceUseCgroup) {
		return CpuMetrics{}, false
	}
	cur, ok := readContainerCpuSample()
	if !ok {
		return CpuMetrics{}, false
	}
	cur.at = cpuNow()

	prev, ok := lastCgroupCpuSamples[cacheTimeMs]
	if !ok {
		prev = lastCgroupCpuSamples[60000]
	}
	lastCgroupCpuSamples[cacheTimeMs] = cur

	// No baseline yet, a backwards counter (cgroup recreated), or a
	// non-positive clock delta: report zero this tick instead of guessing.
	elapsedUsec := cur.at.Sub(prev.at).Microseconds()
	if prev.at.IsZero() || elapsedUsec <= 0 || cur.usageUsec < prev.usageUsec {
		return CpuMetrics{}, true
	}

	cores := cur.cores
	if cores <= 0 {
		cores = 1
	}
	window := float64(elapsedUsec) * cores

	metrics := CpuMetrics{
		Total:  clampPercent(float64(cur.usageUsec-prev.usageUsec) / window * 100),
		User:   clampPercent(float64(cur.userUsec-prev.userUsec) / window * 100),
		System: clampPercent(float64(cur.systemUsec-prev.systemUsec) / window * 100),
	}
	// cgroup accounting has no iowait/steal; everything not busy is idle.
	metrics.Idle = clampPercent(100 - metrics.Total)
	return metrics, true
}

// readContainerCpuSample reads the container's cumulative CPU usage, preferring
// the cgroup v2 unified hierarchy and falling back to the v1 cpuacct
// controller.
func readContainerCpuSample() (cgroupCpuSample, bool) {
	if s, ok := readCgroupV2CpuSample(); ok {
		return s, true
	}
	return readCgroupV1CpuSample()
}

// readCgroupV2CpuSample reads usage from the unified hierarchy's cpu.stat.
//
// The mount root is always the cgroup to read: an LXC guest has a private
// cgroup namespace, so /sys/fs/cgroup already is the guest's root cgroup, and
// its cpu.stat accounts for every process in the guest. The agent's own path
// in /proc/self/cgroup (its service cgroup, or the ".lxc" leaf when started
// from an attached shell) only covers a subset and must not be descended into.
func readCgroupV2CpuSample() (cgroupCpuSample, bool) {
	if !inCgroupV2() {
		return cgroupCpuSample{}, false // no v2 membership; try v1
	}
	dir := cpuCgroupRoot
	if mount := cgroupMountPoint("cgroup2", ""); mount != "" {
		dir = mount
	}
	stat := filepath.Join(dir, "cpu.stat")
	usage, ok := cgroupStatValue(stat, "usage_usec")
	if !ok {
		return cgroupCpuSample{}, false
	}
	s := cgroupCpuSample{usageUsec: usage, cores: cpuCgroupCores(dir)}
	s.userUsec, _ = cgroupStatValue(stat, "user_usec")
	s.systemUsec, _ = cgroupStatValue(stat, "system_usec")
	return s, true
}

// readCgroupV1CpuSample reads usage from the legacy cpuacct controller.
// As with v2, the hierarchy mount root is the guest's own cgroup and its
// accounting includes every child cgroup, so it is read directly rather than
// the agent's own sub-cgroup.
func readCgroupV1CpuSample() (cgroupCpuSample, bool) {
	dir := cgroupMountPoint("cgroup", "cpuacct")
	if dir == "" {
		return cgroupCpuSample{}, false
	}
	usageNs, ok := utils.ReadUintFile(filepath.Join(dir, "cpuacct.usage"))
	if !ok {
		return cgroupCpuSample{}, false
	}
	s := cgroupCpuSample{usageUsec: usageNs / 1000, cores: cpuCgroupCores(dir)}
	// cpuacct.stat reports user/system in USER_HZ jiffies.
	if v, ok := cgroupStatValue(filepath.Join(dir, "cpuacct.stat"), "user"); ok {
		s.userUsec = v * 1e6 / cpuUserHZ
	}
	if v, ok := cgroupStatValue(filepath.Join(dir, "cpuacct.stat"), "system"); ok {
		s.systemUsec = v * 1e6 / cpuUserHZ
	}
	return s, true
}

// inCgroupV2 reports whether /proc/self/cgroup lists the v2 unified hierarchy
// (a "0::<path>" entry).
func inCgroupV2() bool {
	data, err := os.ReadFile(cpuProcSelfCgroup)
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "0::") {
			return true
		}
	}
	return false
}

// cgroupMountPoint returns the mount point of a cgroup hierarchy from
// /proc/self/mountinfo: the cgroup2 mount for v2, or the cgroup mount whose
// super options list the wanted v1 controller.
func cgroupMountPoint(fstype, v1ctrl string) string {
	data, err := os.ReadFile(cpuCgroupMountinfo)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		left, right, found := strings.Cut(line, " - ")
		if !found {
			continue
		}
		post := strings.Fields(right)
		if len(post) == 0 || post[0] != fstype {
			continue
		}
		if v1ctrl != "" && !mountOptHas(post, v1ctrl) {
			continue
		}
		fields := strings.Fields(left)
		if len(fields) >= 5 {
			return unescapeMountPoint(fields[4])
		}
	}
	return ""
}

// mountOptHas reports whether the comma-separated super options (field 3 after
// the " - " separator) contain opt.
func mountOptHas(post []string, opt string) bool {
	if len(post) < 3 {
		return false
	}
	for _, o := range strings.Split(post[2], ",") {
		if o == opt {
			return true
		}
	}
	return false
}

// unescapeMountPoint decodes octal escapes (e.g. \040 for space) used in
// mountinfo paths.
func unescapeMountPoint(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}

// cpuCgroupCores returns how many CPU cores the cgroup at dir may use: the
// smallest of the process affinity mask, the cgroup cpuset, and the CPU quota.
func cpuCgroupCores(dir string) float64 {
	cores := float64(cpuNumCPU())
	if n := cpusetCount(dir); n > 0 && n < cores {
		cores = n
	}
	if q, ok := cpuQuotaCores(dir); ok && q < cores {
		cores = q
	}
	if cores <= 0 {
		cores = 1
	}
	return cores
}

// cpusetCount returns the number of CPUs in the cgroup's cpuset, e.g. "0-3" or
// "2,5-7". An empty or missing file means unconstrained.
func cpusetCount(dir string) float64 {
	for _, name := range []string{"cpuset.cpus.effective", "cpuset.cpus"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if n := countCpuList(strings.TrimSpace(string(raw))); n > 0 {
			return float64(n)
		}
	}
	return 0
}

// countCpuList counts the CPUs in a Linux CPU list like "0-3,5,8-9".
func countCpuList(list string) int {
	total := 0
	for part := range strings.SplitSeq(list, ",") {
		lo, hi, ranged := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			continue
		}
		b := a
		if ranged {
			if v, err := strconv.Atoi(hi); err == nil {
				b = v
			}
		}
		if b >= a {
			total += b - a + 1
		}
	}
	return total
}

// cpuQuotaCores returns the cgroup's CPU quota in cores. v2 uses cpu.max
// ("<quota|max> <period>"), v1 uses cpu.cfs_quota_us / cpu.cfs_period_us.
func cpuQuotaCores(dir string) (float64, bool) {
	if raw, err := os.ReadFile(filepath.Join(dir, "cpu.max")); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) == 2 && fields[0] != "max" {
			if quota, err := strconv.ParseFloat(fields[0], 64); err == nil && quota > 0 {
				if period, err := strconv.ParseFloat(fields[1], 64); err == nil && period > 0 {
					return quota / period, true
				}
			}
		}
	}
	if quota, ok := readCgroupInt(filepath.Join(dir, "cpu.cfs_quota_us")); ok && quota > 0 {
		if period, ok := readCgroupInt(filepath.Join(dir, "cpu.cfs_period_us")); ok && period > 0 {
			return float64(quota) / float64(period), true
		}
	}
	return 0, false
}

// cgroupStatValue returns the value of key in a cgroup "key value" stat file.
func cgroupStatValue(path, key string) (uint64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		name, value, found := strings.Cut(line, " ")
		if !found || name != key {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		return v, err == nil
	}
	return 0, false
}

// readCgroupInt reads a file containing a single signed integer
// (cpu.cfs_quota_us is -1 when no quota is set).
func readCgroupInt(path string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return v, err == nil
}
