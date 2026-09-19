//go:build linux

package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
)

// Container-aware CPU accounting (issue #2332).
//
// Inside a container /proc/stat does not describe the container's own usage:
// lxcfs serves LXC guests the raw counters of the host cores in their cpuset,
// and plain runtimes (Docker, k8s) expose the host's /proc outright. An idle
// container sharing a host core with a busy neighbor then reports near-100%
// CPU while doing nothing. The cgroup's own accounting (cpu.stat /
// cpuacct.usage) reflects only the container's processes, so when the agent
// runs inside a container we derive CPU% from that instead.

// File paths and hooks are variables so tests can point them at fixtures.
var (
	cpuCgroupRoot      = "/sys/fs/cgroup" // default cgroup v2 mount point
	cpuCgroupMountinfo = "/proc/self/mountinfo"
	cpuProcSelfCgroup  = "/proc/self/cgroup"
	cpuProcOneEnviron  = "/proc/1/environ"
	cpuDockerenvPath   = "/.dockerenv"
	cpuContainerenv    = "/run/.containerenv"
	cpuSystemdContPath = "/run/systemd/container"
	cpuNumCPU          = runtime.NumCPU
	cpuNow             = time.Now
)

// cpuUserHZ is the USER_HZ jiffies-per-second rate cpuacct.stat reports in.
const cpuUserHZ = 100

var (
	containerOnce     sync.Once
	containerDetected bool
)

// inContainer reports whether the agent itself runs inside a container.
// The result is cached because it cannot change during the process lifetime.
func inContainer() bool {
	containerOnce.Do(func() { containerDetected = detectContainer() })
	return containerDetected
}

// detectContainer looks for the usual container markers.
func detectContainer() bool {
	// set by systemd-nspawn, LXC, Podman and others
	if os.Getenv("container") != "" {
		return true
	}
	for _, p := range []string{cpuDockerenvPath, cpuContainerenv, cpuSystemdContPath} {
		if utils.FileExists(p) {
			return true
		}
	}
	// liblxc always puts container=lxc in the container init's environment,
	// which survives on non-systemd guests such as Alpine LXC.
	if data, err := os.ReadFile(cpuProcOneEnviron); err == nil {
		if bytes.HasPrefix(data, []byte("container=")) ||
			bytes.Contains(data, []byte("\x00container=")) {
			return true
		}
	}
	// an lxcfs mount means /proc/stat is virtualized with host core counters
	if data, err := os.ReadFile(cpuCgroupMountinfo); err == nil &&
		bytes.Contains(data, []byte(" - fuse.lxcfs ")) {
		return true
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

// init seeds the container CPU baseline so the first reported value is a real
// delta since startup rather than zero.
func init() {
	if !inContainer() {
		return
	}
	if s, ok := readContainerCpuSample(); ok {
		s.at = cpuNow()
		lastCgroupCpuSamples[60000] = s
	}
}

// containerCpuMetrics derives CPU metrics from the agent's own cgroup
// accounting when running inside a container. It returns ok=false on plain
// hosts and whenever cgroup accounting is unreadable, so callers keep the
// /proc/stat fallback.
func containerCpuMetrics(cacheTimeMs uint16) (CpuMetrics, bool) {
	if !inContainer() {
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
// The mount root is usually the right cgroup to read: inside a private cgroup
// namespace (LXC, default Docker) /sys/fs/cgroup already is the container's
// root cgroup, and its cpu.stat accounts for every process in the container,
// including siblings of the agent's own service cgroup. When the hierarchy is
// not namespaced (e.g. docker run --cgroupns=host) /proc/self/cgroup instead
// holds the container's real host-side path, which is joined onto the mount.
func readCgroupV2CpuSample() (cgroupCpuSample, bool) {
	rel := selfCgroupPath("0::")
	if rel == "" {
		return cgroupCpuSample{}, false // no v2 membership; try v1
	}
	dir := cpuCgroupRoot
	if mount := cgroupMountPoint("cgroup2", ""); mount != "" {
		dir = mount
	}
	if rel != "/" && hasContainerRuntimeMarker(rel) {
		if cand := filepath.Join(dir, rel); directoryExistsOK(cand) {
			dir = cand
		}
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
// Runtimes bind-mount the container's own cpuacct directory at the hierarchy
// mount, so the mount root is normally already the container's cgroup; if the
// process's cgroup path still resolves below the mount (shared host view),
// that subdirectory is used instead.
func readCgroupV1CpuSample() (cgroupCpuSample, bool) {
	mount := cgroupMountPoint("cgroup", "cpuacct")
	if mount == "" {
		return cgroupCpuSample{}, false
	}
	dir := mount
	if rel := selfCgroupPath("cpuacct"); rel != "" && rel != "/" {
		if cand := filepath.Join(mount, rel); utils.FileExists(filepath.Join(cand, "cpuacct.usage")) {
			dir = cand
		}
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

// selfCgroupPath returns the agent's cgroup path from /proc/self/cgroup: the
// path after "0::" for the v2 unified hierarchy, or the path of the entry
// whose controller list contains the given v1 controller (e.g. "cpuacct").
func selfCgroupPath(selector string) string {
	data, err := os.ReadFile(cpuProcSelfCgroup)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		if selector == "0::" {
			if parts[0] == "0" && parts[1] == "" {
				return parts[2]
			}
			continue
		}
		for _, ctrl := range strings.Split(parts[1], ",") {
			if ctrl == selector {
				return parts[2]
			}
		}
	}
	return ""
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

// hasContainerRuntimeMarker reports whether a cgroup path looks like a real
// host-side container cgroup path (docker/k8s/lxc/podman), meaning the visible
// hierarchy is not namespaced and the path can be resolved under the mount.
func hasContainerRuntimeMarker(path string) bool {
	for _, m := range []string{"docker", "kubepods", "lxc", "crio", "libpod", "containerd", "podman"} {
		if strings.Contains(path, m) {
			return true
		}
	}
	return false
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
	for _, part := range strings.Split(list, ",") {
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
	for _, line := range strings.Split(string(data), "\n") {
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

// directoryExistsOK reports whether path is a directory.
func directoryExistsOK(path string) bool {
	ok, _ := directoryExists(path)
	return ok
}
