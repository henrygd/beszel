package agent

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	// gpuClientsRescanInterval is how long before every fd of a known process
	// is resolved again, to catch a DRM fd that a change check missed.
	gpuClientsRescanInterval = 5 * time.Minute
	// gpuClientsFallbackInterval is how long before the fds of a known process
	// are listed again on kernels that don't report the number of open fds.
	gpuClientsFallbackInterval = 10 * time.Second
	// gpuClientsRelistBackoff is the minimum time between listings when the
	// number of open fds changes, so processes that churn fds stay cheap.
	gpuClientsRelistBackoff = 5 * time.Second
	// gpuClientsStartupWindow is how long after a process is first seen its fds
	// are listed on every scan. A process is often seen before it has opened
	// the GPU, or before it has exec'd and has its final name.
	gpuClientsStartupWindow = 5 * time.Second

	// GPU vendors, used to attribute DRM clients to GPUs whose collector id
	// is not a sysfs card id.
	gpuVendorIntel  = "intel"
	gpuVendorAmd    = "amd"
	gpuVendorNvidia = "nvidia"
)

// gpuClientTracker attributes GPU engine usage to containers, systemd services
// and processes by reading DRM fdinfo (/proc/<pid>/fdinfo). This is the kernel
// interface used by intel_gpu_top and nvtop for per-client stats, supported by
// i915, xe, amdgpu and other DRM drivers (not the proprietary NVIDIA driver).
//
// Reading the fdinfo of a process owned by another user needs CAP_SYS_PTRACE.
// The agent can get it without running as root: AmbientCapabilities in the
// systemd unit, or cap_add with pid: host in Docker.
//
// https://docs.kernel.org/gpu/drm-usage-stats.html
type gpuClientTracker struct {
	procRoot string
	sysRoot  string
	now      func() time.Time
	// fdCount returns the number of open fds of a process, or nil if the kernel
	// doesn't report it (before Linux 6.2).
	fdCount func(pid string) int64
	// containerName resolves a short container id to its name. May be nil.
	containerName func(shortId string) string

	mu    sync.Mutex
	prev  map[uint16]*drmSnapshot // cacheTimeMs -> previous snapshot
	procs map[string]*drmProc     // pid -> cached process info
	scans int

	warnedPermission bool
}

// drmProc caches what a process is and which of its fds are DRM clients.
type drmProc struct {
	consumer string
	fds      []string            // fds pointing to a DRM device
	otherFds map[string]struct{} // other fds, not resolved again until the next full listing
	fdCount  int64               // number of open fds at the last listing
	listedAt time.Time           // when fd/ was last listed; zero to list on the next scan
	// fullAt is when every fd was last resolved, ignoring otherFds
	fullAt    time.Time
	firstSeen time.Time
	firstScan int // scan number when the process was first seen
}

// drmClientKey identifies a DRM client. The same client can be shared by
// several file descriptors and processes, so it is deduplicated by this key.
type drmClientKey struct {
	device string // drm-pdev, or drm-driver if the driver doesn't report it
	id     uint64 // drm-client-id
}

type drmClient struct {
	consumer string
	engines  map[string]drmEngine
	// procScan is the scan in which the oldest process sharing the client was
	// first seen. A client from a process that is new since the previous
	// snapshot has no earlier busy time, so it can start from zero.
	procScan int
}

// drmEngine holds the cumulative counters of one engine class for a client.
// Drivers report either busy time in ns (i915, amdgpu) or cycles (xe).
type drmEngine struct {
	busyNs      uint64
	cycles      uint64
	totalCycles uint64
	capacity    uint64
}

type drmSnapshot struct {
	time    time.Time
	scan    int
	clients map[drmClientKey]drmClient
}

// newGpuClientTracker returns a tracker on Linux when a GPU with per-client
// DRM stats is present.
func newGpuClientTracker() *gpuClientTracker {
	if runtime.GOOS != "linux" {
		return nil
	}
	tracker := &gpuClientTracker{procRoot: "/proc", sysRoot: "/sys"}
	if !tracker.hasDrmClientStats() {
		return nil
	}
	if info, err := os.Stat("/proc/self/fd"); err == nil && info.Size() > 0 {
		tracker.fdCount = tracker.procFdCount
	}
	if os.Getpid() == 1 {
		slog.Info("GPU processes: only the agent's own processes are visible; run the agent container with pid: host")
	}
	return tracker
}

// hasDrmClientStats reports whether a DRM card uses a driver other than the
// proprietary NVIDIA one, which doesn't report per-client stats in fdinfo.
func (t *gpuClientTracker) hasDrmClientStats() bool {
	drivers, _ := filepath.Glob(filepath.Join(t.sysRoot, "class/drm/card[0-9]*/device/driver"))
	for _, driver := range drivers {
		if link, err := os.Readlink(driver); err == nil && filepath.Base(link) != "nvidia" {
			return true
		}
	}
	return false
}

// collect returns GPU usage percent per consumer, keyed by DRM device
// (drm-pdev). The usage of a consumer is its busiest engine class, summed
// across its clients. The first call for a cache time only records a baseline.
func (t *gpuClientTracker) collect(cacheTimeMs uint16) map[string]map[string]float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now
	if t.now != nil {
		now = t.now
	}
	ts := now()
	clients := t.scan(ts)
	snap := &drmSnapshot{time: ts, scan: t.scans, clients: clients}

	if t.prev == nil {
		t.prev = make(map[uint16]*drmSnapshot)
	}
	prev := t.prev[cacheTimeMs]
	t.prev[cacheTimeMs] = snap
	if prev == nil {
		return nil
	}
	return drmUsageByConsumer(prev, snap)
}

// drmUsageByConsumer computes per-device, per-consumer usage between two snapshots.
func drmUsageByConsumer(prev, cur *drmSnapshot) map[string]map[string]float64 {
	elapsedNs := cur.time.Sub(prev.time).Nanoseconds()
	if elapsedNs <= 0 {
		return nil
	}

	type engineKey struct {
		device   string
		consumer string
		engine   string
	}
	busy := make(map[engineKey]float64)

	for key, client := range cur.clients {
		prevClient, existed := prev.clients[key]
		for name, engine := range client.engines {
			prevEngine := prevClient.engines[name]
			var fraction float64
			switch {
			case engine.totalCycles > 0:
				// total-cycles is a GPU-wide clock, so a new client has no baseline
				if !existed || engine.cycles < prevEngine.cycles || engine.totalCycles <= prevEngine.totalCycles {
					continue
				}
				fraction = float64(engine.cycles-prevEngine.cycles) / float64(engine.totalCycles-prevEngine.totalCycles)
			default:
				// A new client of a process created since the previous snapshot
				// accumulated all of its busy time within this interval, so a zero
				// baseline is correct. A process seen before may have opened the
				// GPU long before its fds were listed again, so it only gets a
				// baseline.
				if !existed && client.procScan <= prev.scan {
					continue
				}
				if engine.busyNs < prevEngine.busyNs {
					continue
				}
				fraction = float64(engine.busyNs-prevEngine.busyNs) / float64(elapsedNs)
			}
			if engine.capacity > 1 {
				fraction /= float64(engine.capacity)
			}
			busy[engineKey{key.device, client.consumer, name}] += fraction
		}
	}

	usage := make(map[string]map[string]float64)
	for key, fraction := range busy {
		pct := utils.TwoDecimals(math.Min(fraction*100, 100))
		if pct <= 0 {
			continue
		}
		if usage[key.device] == nil {
			usage[key.device] = make(map[string]float64)
		}
		if pct > usage[key.device][key.consumer] {
			usage[key.device][key.consumer] = pct
		}
	}
	for device, clients := range usage {
		usage[device] = system.TrimGPUClients(clients, system.MaxGPUClients)
	}
	return usage
}

// scan reads the DRM clients of all visible processes. See needsList for
// when the fds of a process are listed.
func (t *gpuClientTracker) scan(now time.Time) map[drmClientKey]drmClient {
	clients := make(map[drmClientKey]drmClient)
	entries, err := os.ReadDir(t.procRoot)
	if err != nil {
		slog.Debug("GPU processes: read proc", "err", err)
		return clients
	}
	if t.procs == nil {
		t.procs = make(map[string]*drmProc)
	}
	t.scans++

	var denied int
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		pid := entry.Name()
		if !entry.IsDir() || !isNumeric(pid) {
			continue
		}
		seen[pid] = struct{}{}

		proc := t.procs[pid]
		if proc == nil {
			consumer, ok := t.consumerForPid(pid)
			if !ok {
				continue
			}
			proc = &drmProc{consumer: consumer, firstSeen: now, firstScan: t.scans}
			t.procs[pid] = proc
		} else if t.needsList(proc, pid, now) {
			// a process may have exec'd or moved cgroup since it was first seen
			if consumer, ok := t.consumerForPid(pid); ok {
				proc.consumer = consumer
			}
			proc.listedAt = time.Time{}
		}
		if proc.listedAt.IsZero() {
			if t.fdCount != nil {
				proc.fdCount = t.fdCount(pid)
			}
			full := now.Sub(proc.fullAt) >= gpuClientsRescanInterval
			err := t.listFds(pid, proc, full)
			proc.listedAt = now
			if full {
				proc.fullAt = now
			}
			if err != nil {
				if errors.Is(err, fs.ErrPermission) {
					denied++
				}
				continue
			}
		}

		for _, fd := range proc.fds {
			data, err := os.ReadFile(filepath.Join(t.procRoot, pid, "fdinfo", fd))
			if err != nil {
				// fd was closed; list fds again next scan
				proc.listedAt = time.Time{}
				continue
			}
			key, engines, ok := parseDrmFdinfo(data)
			if !ok {
				continue
			}
			if client, dup := clients[key]; dup {
				if proc.firstScan < client.procScan {
					client.procScan = proc.firstScan
					clients[key] = client
				}
				continue
			}
			clients[key] = drmClient{consumer: t.consumerName(proc.consumer), engines: engines, procScan: proc.firstScan}
		}
	}

	for pid := range t.procs {
		if _, ok := seen[pid]; !ok {
			delete(t.procs, pid)
		}
	}

	if denied > 0 && !t.warnedPermission {
		t.warnedPermission = true
		slog.Info("GPU processes: can't read other users' processes; grant CAP_SYS_PTRACE to show them (systemd: AmbientCapabilities=CAP_SYS_PTRACE, Docker: cap_add SYS_PTRACE)", "processes", denied)
	}
	return clients
}

// needsList reports whether the fds of a known process should be listed:
// on every scan while it starts up, when its number of open fds changes, and
// every gpuClientsRescanInterval to catch an fd swapped for another one.
// Without fd counts, every gpuClientsFallbackInterval instead.
func (t *gpuClientTracker) needsList(proc *drmProc, pid string, now time.Time) bool {
	sinceListed := now.Sub(proc.listedAt)
	switch {
	case proc.listedAt.IsZero(), now.Sub(proc.fullAt) >= gpuClientsRescanInterval:
		return true
	case t.fdCount == nil:
		return now.Sub(proc.firstSeen) < gpuClientsStartupWindow || sinceListed >= gpuClientsFallbackInterval
	case t.fdCount(pid) == proc.fdCount:
		return false
	default:
		return now.Sub(proc.firstSeen) < gpuClientsStartupWindow || sinceListed >= gpuClientsRelistBackoff
	}
}

// listFds updates the DRM fds of a process. Only fd numbers that weren't open
// at the previous listing are resolved, unless full is set.
func (t *gpuClientTracker) listFds(pid string, proc *drmProc, full bool) error {
	fdDir := filepath.Join(t.procRoot, pid, "fd")
	dir, err := os.Open(fdDir)
	if err != nil {
		proc.fds, proc.otherFds = nil, nil
		return err
	}
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		proc.fds, proc.otherFds = nil, nil
		return err
	}

	var fds []string
	others := make(map[string]struct{}, len(names))
	for _, fd := range names {
		if _, known := proc.otherFds[fd]; known && !full {
			others[fd] = struct{}{}
			continue
		}
		link, err := os.Readlink(filepath.Join(fdDir, fd))
		if err != nil {
			// listing fd/ is allowed without ptrace access, but readlink is not
			if errors.Is(err, fs.ErrPermission) {
				proc.fds, proc.otherFds = nil, nil
				return err
			}
			continue
		}
		if strings.HasPrefix(link, "/dev/dri/") {
			fds = append(fds, fd)
		} else {
			others[fd] = struct{}{}
		}
	}
	proc.fds, proc.otherFds = fds, others
	return nil
}

// procFdCount returns the number of open fds of a process, which Linux 6.2+
// reports as the size of /proc/<pid>/fd. Returns -1 if it can't be read.
func (t *gpuClientTracker) procFdCount(pid string) int64 {
	info, err := os.Stat(filepath.Join(t.procRoot, pid, "fd"))
	if err != nil {
		return -1
	}
	return info.Size()
}

// consumerForPid names what a process belongs to, from files readable without
// ptrace access: "c:<short container id>", "s:<service name>" or "p:<process name>".
func (t *gpuClientTracker) consumerForPid(pid string) (string, bool) {
	cgroup, err := os.ReadFile(filepath.Join(t.procRoot, pid, "cgroup"))
	if err != nil {
		return "", false
	}
	if id := containerIdFromCgroup(cgroup); id != "" {
		return "c:" + id[:12], true
	}
	if unit := serviceFromCgroup(cgroup); unit != "" {
		return "s:" + unit, true
	}
	comm, err := os.ReadFile(filepath.Join(t.procRoot, pid, "comm"))
	if err != nil {
		return "", false
	}
	return "p:" + strings.TrimSpace(string(comm)), true
}

// consumerName replaces a container's short id with its name when known.
// Names are resolved on each scan since a new container may not be known yet
// when its process is first seen.
func (t *gpuClientTracker) consumerName(consumer string) string {
	if shortId, ok := strings.CutPrefix(consumer, "c:"); ok && t.containerName != nil {
		if name := t.containerName(shortId); name != "" {
			return "c:" + name
		}
	}
	return consumer
}

// gpuIdForDevice maps a drm-pdev PCI address to the card id used by the sysfs
// GPU collectors (e.g. "card0"). Returns "" if it can't be resolved.
func (t *gpuClientTracker) gpuIdForDevice(device string) string {
	matches, _ := filepath.Glob(filepath.Join(t.sysRoot, "bus/pci/devices", device, "drm", "card*"))
	for _, match := range matches {
		name := filepath.Base(match)
		if isNumeric(strings.TrimPrefix(name, "card")) {
			return name
		}
	}
	return ""
}

// attachGpuClients sets Clients on the GPU each device belongs to. A device
// that doesn't match a sysfs card id (e.g. intel_gpu_top reports "i0") is
// attached to the only GPU of its vendor, if it's the only card of that vendor.
// gpuVendors maps GPU ids to the vendor reported by their collector.
func (t *gpuClientTracker) attachGpuClients(gpuData map[string]system.GPUData, gpuVendors map[string]string, usage map[string]map[string]float64) {
	for device, clients := range usage {
		id := t.gpuIdForDevice(device)
		if _, ok := gpuData[id]; !ok || id == "" {
			id = t.gpuIdForVendor(gpuData, gpuVendors, drmDriverVendor(t.driverForDevice(device)))
		}
		if gpu, ok := gpuData[id]; ok && id != "" && gpu.Clients == nil {
			gpu.Clients = clients
			gpuData[id] = gpu
		}
	}
}

// gpuIdForVendor returns the id of the only GPU from vendor, or "" if there is
// not exactly one, or the system has more than one card from that vendor.
func (t *gpuClientTracker) gpuIdForVendor(gpuData map[string]system.GPUData, gpuVendors map[string]string, vendor string) string {
	if vendor == "" || t.cardsForVendor(vendor) != 1 {
		return ""
	}
	var match string
	for id := range gpuData {
		if gpuVendors[id] == vendor {
			if match != "" {
				return ""
			}
			match = id
		}
	}
	return match
}

// cardsForVendor counts the DRM cards whose driver belongs to vendor.
func (t *gpuClientTracker) cardsForVendor(vendor string) int {
	drivers, _ := filepath.Glob(filepath.Join(t.sysRoot, "class/drm/card[0-9]*/device/driver"))
	var n int
	for _, driver := range drivers {
		if link, err := os.Readlink(driver); err == nil && drmDriverVendor(filepath.Base(link)) == vendor {
			n++
		}
	}
	return n
}

// driverForDevice returns the kernel driver of a drm-pdev PCI address. A device
// without drm-pdev is keyed by its driver name, which is returned as is.
func (t *gpuClientTracker) driverForDevice(device string) string {
	link, err := os.Readlink(filepath.Join(t.sysRoot, "bus/pci/devices", device, "driver"))
	if err != nil {
		return device
	}
	return filepath.Base(link)
}

// drmDriverVendor returns the GPU vendor of a DRM driver, or "" if unknown.
func drmDriverVendor(driver string) string {
	switch driver {
	case "i915", "xe":
		return gpuVendorIntel
	case "amdgpu", "radeon":
		return gpuVendorAmd
	case "nouveau", "nvidia":
		return gpuVendorNvidia
	}
	return ""
}

// parseDrmFdinfo parses the drm-* keys of a DRM file descriptor's fdinfo.
// It returns false if the fd is not a DRM client.
func parseDrmFdinfo(data []byte) (key drmClientKey, engines map[string]drmEngine, ok bool) {
	var driver string
	var hasId bool
	engines = make(map[string]drmEngine)
	update := func(name string, fn func(*drmEngine)) {
		e := engines[name]
		fn(&e)
		engines[name] = e
	}

	for line := range bytes.Lines(data) {
		k, v, found := strings.Cut(string(line), ":")
		if !found || !strings.HasPrefix(k, "drm-") {
			continue
		}
		v = strings.TrimSpace(v)
		switch {
		case k == "drm-driver":
			driver = v
		case k == "drm-pdev":
			key.device = v
		case k == "drm-client-id":
			key.id, _ = strconv.ParseUint(v, 10, 64)
			hasId = true
		case strings.HasPrefix(k, "drm-engine-capacity-"):
			n := parseDrmValue(v)
			update(strings.TrimPrefix(k, "drm-engine-capacity-"), func(e *drmEngine) { e.capacity = n })
		case strings.HasPrefix(k, "drm-engine-"):
			n := parseDrmValue(v)
			update(strings.TrimPrefix(k, "drm-engine-"), func(e *drmEngine) { e.busyNs = n })
		case strings.HasPrefix(k, "drm-total-cycles-"):
			n := parseDrmValue(v)
			update(strings.TrimPrefix(k, "drm-total-cycles-"), func(e *drmEngine) { e.totalCycles = n })
		case strings.HasPrefix(k, "drm-cycles-"):
			n := parseDrmValue(v)
			update(strings.TrimPrefix(k, "drm-cycles-"), func(e *drmEngine) { e.cycles = n })
		}
	}

	if driver == "" || !hasId {
		return key, nil, false
	}
	if key.device == "" {
		key.device = driver
	}
	return key, engines, true
}

// parseDrmValue parses a value such as "220198108 ns" or "2".
func parseDrmValue(v string) uint64 {
	v, _, _ = strings.Cut(v, " ")
	n, _ := strconv.ParseUint(v, 10, 64)
	return n
}

// containerIdFromCgroup returns the last 64-character hex id in a
// /proc/<pid>/cgroup file, which covers Docker and Podman with either the
// systemd or cgroupfs driver (e.g. "0::/system.slice/docker-<id>.scope",
// "0::/docker/<id>", "0::/../libpod-<id>.scope").
func containerIdFromCgroup(data []byte) string {
	var id string
	start := -1
	for i := 0; i <= len(data); i++ {
		if i < len(data) && isHexByte(data[i]) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start == 64 {
			id = string(data[start:i])
		}
		start = -1
	}
	return id
}

// serviceFromCgroup returns the name of the system service a process belongs
// to, from the unified (cgroup v2) line, e.g. "jellyfin" for
// "0::/system.slice/jellyfin.service". Per-user
// service managers (user@1000.service) are skipped so desktop apps are named
// by process instead.
func serviceFromCgroup(data []byte) string {
	for line := range bytes.Lines(data) {
		path, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("0::"))
		if !ok {
			continue
		}
		var unit string
		for segment := range strings.SplitSeq(string(path), "/") {
			if name, ok := strings.CutSuffix(segment, ".service"); ok {
				if strings.HasPrefix(name, "user@") {
					return ""
				}
				unit = name
			}
		}
		return unit
	}
	return ""
}

func isHexByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
