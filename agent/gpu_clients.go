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
	// gpuClientsRescanInterval is how many scans pass before the fds of a known
	// process are listed again, to catch processes that open the GPU later.
	gpuClientsRescanInterval = 10
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
	fds      []string
	listedAt int // scan number when fd/ was last listed
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
	snap := &drmSnapshot{time: now(), clients: t.scan()}

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
				// A client created since the previous scan accumulated all of its
				// busy time within this interval, so a zero baseline is correct.
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

// scan reads the DRM clients of all visible processes. New processes have
// their fds listed right away; known processes every gpuClientsRescanInterval
// scans, or when one of their DRM fds disappears.
func (t *gpuClientTracker) scan() map[drmClientKey]drmClient {
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
			proc = &drmProc{consumer: consumer}
			t.procs[pid] = proc
		}
		if proc.listedAt == 0 || t.scans-proc.listedAt >= gpuClientsRescanInterval {
			fds, err := t.drmFds(pid)
			proc.listedAt = t.scans
			proc.fds = fds
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
				proc.listedAt = t.scans - gpuClientsRescanInterval
				continue
			}
			key, engines, ok := parseDrmFdinfo(data)
			if !ok {
				continue
			}
			if _, dup := clients[key]; !dup {
				clients[key] = drmClient{consumer: t.consumerName(proc.consumer), engines: engines}
			}
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

// drmFds returns the fds of a process that point to a DRM device.
func (t *gpuClientTracker) drmFds(pid string) ([]string, error) {
	fdDir := filepath.Join(t.procRoot, pid, "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, err
	}
	var fds []string
	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			// listing fd/ is allowed without ptrace access, but readlink is not
			if errors.Is(err, fs.ErrPermission) {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(link, "/dev/dri/") {
			fds = append(fds, entry.Name())
		}
	}
	return fds, nil
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

// attachGpuClients sets Clients on the GPU each device belongs to. Devices that
// don't match a GPU id are attached to the only GPU if there is exactly one.
func (t *gpuClientTracker) attachGpuClients(gpuData map[string]system.GPUData, usage map[string]map[string]float64) {
	var unmatched []map[string]float64
	for device, clients := range usage {
		id := t.gpuIdForDevice(device)
		if gpu, ok := gpuData[id]; ok && id != "" {
			gpu.Clients = clients
			gpuData[id] = gpu
			continue
		}
		unmatched = append(unmatched, clients)
	}
	if len(unmatched) == 1 && len(gpuData) == 1 {
		for id, gpu := range gpuData {
			if gpu.Clients == nil {
				gpu.Clients = unmatched[0]
				gpuData[id] = gpu
			}
		}
	}
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
