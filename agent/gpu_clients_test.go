//go:build testing

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testJellyfinId = "1484086dc55a606e9531d283628c14d5141c607fdfbc0db99f703416c223c301"
	testImmichId   = "8f1c2b3a4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f8"
	testPdev       = "0000:00:02.0"
	testRenderNode = "/dev/dri/renderD128"
)

// i915 fdinfo captured from a VAAPI h264 encode on an Alder Lake-N iGPU.
func i915Fdinfo(clientId int, render, video uint64) string {
	return fmt.Sprintf(`pos:	0
flags:	02100002
mnt_id:	29
ino:	1055
drm-driver:	i915
drm-client-id:	%d
drm-pdev:	0000:00:02.0
drm-total-system0:	84624 KiB
drm-resident-system0:	75700 KiB
drm-engine-render:	%d ns
drm-engine-copy:	0 ns
drm-engine-video:	%d ns
drm-engine-video-enhance:	0 ns
`, clientId, render, video)
}

type fakeProc struct {
	t    *testing.T
	root string
}

func newFakeProc(t *testing.T) *fakeProc {
	return &fakeProc{t: t, root: t.TempDir()}
}

// addProcess creates /proc/<pid> with cgroup and comm files and fds pointing
// at the given targets. fdinfo holds the fdinfo content per fd (same index).
func (p *fakeProc) addProcess(pid int, comm, cgroup string, targets []string, fdinfo []string) {
	dir := filepath.Join(p.root, fmt.Sprint(pid))
	require.NoError(p.t, os.MkdirAll(filepath.Join(dir, "fd"), 0o755))
	require.NoError(p.t, os.MkdirAll(filepath.Join(dir, "fdinfo"), 0o755))
	require.NoError(p.t, os.WriteFile(filepath.Join(dir, "cgroup"), []byte(cgroup), 0o644))
	require.NoError(p.t, os.WriteFile(filepath.Join(dir, "comm"), []byte(comm+"\n"), 0o644))
	for i, target := range targets {
		p.addFd(pid, i+3, target, fdinfo[i])
	}
}

func (p *fakeProc) addFd(pid, fd int, target, fdinfo string) {
	dir := filepath.Join(p.root, fmt.Sprint(pid))
	require.NoError(p.t, os.Symlink(target, filepath.Join(dir, "fd", fmt.Sprint(fd))))
	p.setFdinfo(pid, fd, fdinfo)
}

func (p *fakeProc) setFdinfo(pid, fd int, content string) {
	path := filepath.Join(p.root, fmt.Sprint(pid), "fdinfo", fmt.Sprint(fd))
	require.NoError(p.t, os.WriteFile(path, []byte(content), 0o644))
}

func (p *fakeProc) removeFd(pid, fd int) {
	dir := filepath.Join(p.root, fmt.Sprint(pid))
	require.NoError(p.t, os.Remove(filepath.Join(dir, "fd", fmt.Sprint(fd))))
	require.NoError(p.t, os.Remove(filepath.Join(dir, "fdinfo", fmt.Sprint(fd))))
}

func dockerCgroup(id string) string {
	return "0::/../docker-" + id + ".scope\n"
}

// newTestTracker returns a tracker on a fake /proc with a clock advanced by tick.
func newTestTracker(proc *fakeProc) (*gpuClientTracker, func(time.Duration)) {
	now := time.Unix(1000, 0)
	tracker := &gpuClientTracker{procRoot: proc.root, now: func() time.Time { return now }}
	return tracker, func(d time.Duration) { now = now.Add(d) }
}

func TestGpuClientTrackerCollect(t *testing.T) {
	proc := newFakeProc(t)

	// ffmpeg in jellyfin, and a forked child sharing the same DRM client
	proc.addProcess(100, "ffmpeg", dockerCgroup(testJellyfinId), []string{"/dev/null", testRenderNode}, []string{"pos: 0\n", i915Fdinfo(179, 1e9, 2e9)})
	proc.addProcess(101, "ffmpeg", dockerCgroup(testJellyfinId), []string{testRenderNode}, []string{i915Fdinfo(179, 1e9, 2e9)})
	// container without a known name
	proc.addProcess(200, "immich", "0::/system.slice/docker-"+testImmichId+".scope\n", []string{testRenderNode}, []string{i915Fdinfo(180, 5e9, 0)})
	// systemd service and a desktop process
	proc.addProcess(300, "ollama", "0::/system.slice/ollama.service\n", []string{testRenderNode}, []string{i915Fdinfo(181, 0, 0)})
	proc.addProcess(400, "firefox", "0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-firefox.scope\n", []string{testRenderNode}, []string{i915Fdinfo(182, 0, 0)})
	// process without GPU fds
	proc.addProcess(500, "sshd", "0::/system.slice/ssh.service\n", []string{"/dev/null"}, []string{"pos: 0\n"})

	tracker, tick := newTestTracker(proc)
	tracker.containerName = func(shortId string) string {
		if shortId == testJellyfinId[:12] {
			return "jellyfin"
		}
		return ""
	}

	assert.Nil(t, tracker.collect(60000), "first collect only records a baseline")

	tick(2 * time.Second)
	// jellyfin: render +220ms, video +400ms over 2s
	proc.setFdinfo(100, 4, i915Fdinfo(179, 1.22e9, 2.4e9))
	proc.setFdinfo(101, 3, i915Fdinfo(179, 1.22e9, 2.4e9))
	// immich: render +1s over 2s
	proc.setFdinfo(200, 3, i915Fdinfo(180, 6e9, 0))
	proc.setFdinfo(300, 3, i915Fdinfo(181, 0.5e9, 0))
	proc.setFdinfo(400, 3, i915Fdinfo(182, 0.1e9, 0))
	// new jellyfin process since the last scan: all of its time is in this interval
	proc.addProcess(102, "ffmpeg", dockerCgroup(testJellyfinId), []string{testRenderNode}, []string{i915Fdinfo(190, 0, 0.2e9)})

	assert.Equal(t, map[string]map[string]float64{
		testPdev: {
			"c:jellyfin":             30, // video 0.4s + 0.2s over 2s, not double counted from pid 101
			"c:" + testImmichId[:12]: 50,
			"s:ollama":               25,
			"p:firefox":              5,
		},
	}, tracker.collect(60000))

	// other cache times keep their own baseline
	assert.Nil(t, tracker.collect(1000))
}

func TestGpuClientTrackerIdleAndCounterReset(t *testing.T) {
	proc := newFakeProc(t)
	proc.addProcess(100, "ffmpeg", dockerCgroup(testJellyfinId), []string{testRenderNode}, []string{i915Fdinfo(179, 5e9, 5e9)})
	proc.addProcess(101, "idle", "0::/system.slice/idle.service\n", []string{testRenderNode}, []string{i915Fdinfo(180, 1e9, 0)})

	tracker, tick := newTestTracker(proc)
	tracker.collect(60000)

	tick(time.Second)
	proc.setFdinfo(100, 3, i915Fdinfo(179, 1e9, 1e9))
	assert.Empty(t, tracker.collect(60000), "idle clients and counters going backwards are left out")
}

func TestGpuClientTrackerFdChanges(t *testing.T) {
	proc := newFakeProc(t)
	proc.addProcess(100, "worker", "0::/system.slice/worker.service\n", []string{"/dev/null"}, []string{"pos: 0\n"})

	tracker, tick := newTestTracker(proc)
	tracker.collect(60000)

	// opening the GPU later is picked up when fds are listed again
	proc.addFd(100, 4, testRenderNode, i915Fdinfo(200, 0, 0))
	tick(gpuClientsFallbackInterval - time.Second)
	assert.Empty(t, tracker.collect(60000))
	assert.Empty(t, tracker.procs["100"].fds)
	tick(time.Second)
	assert.Empty(t, tracker.collect(60000), "baseline for the new client")
	require.Equal(t, []string{"4"}, tracker.procs["100"].fds)

	tick(time.Second)
	proc.setFdinfo(100, 4, i915Fdinfo(200, 0.5e9, 0))
	assert.Equal(t, map[string]float64{"s:worker": 50}, tracker.collect(60000)[testPdev])

	// closing the fd triggers a relist on the next scan; exited processes are dropped
	proc.removeFd(100, 4)
	tick(time.Second)
	assert.Empty(t, tracker.collect(60000))
	tick(time.Second)
	tracker.collect(60000)
	assert.Empty(t, tracker.procs["100"].fds)

	require.NoError(t, os.RemoveAll(filepath.Join(proc.root, "100")))
	tracker.collect(60000)
	assert.NotContains(t, tracker.procs, "100")
}

func TestGpuClientTrackerStartup(t *testing.T) {
	proc := newFakeProc(t)
	tracker, tick := newTestTracker(proc)
	tracker.collect(60000)
	tracker.collect(1000)

	// a shell forks and is seen before it execs ffmpeg, which then opens the GPU
	tick(time.Second)
	proc.addProcess(300, "bash", "0::/user.slice/user-1000.slice/session-2.scope\n", nil, nil)
	assert.Empty(t, tracker.collect(1000))

	tick(time.Second)
	require.NoError(t, os.WriteFile(filepath.Join(proc.root, "300", "comm"), []byte("ffmpeg\n"), 0o644))
	proc.addFd(300, 3, testRenderNode, i915Fdinfo(200, 0, 0.1e9))
	assert.Empty(t, tracker.collect(1000), "the process was seen at the previous realtime scan")

	tick(time.Second)
	proc.setFdinfo(300, 3, i915Fdinfo(200, 0, 0.2e9))
	assert.Equal(t, map[string]float64{"p:ffmpeg": 10}, tracker.collect(1000)[testPdev])
	// but it's new since the 60s snapshot, so all of its time counts there
	assert.Equal(t, map[string]float64{"p:ffmpeg": 6.67}, tracker.collect(60000)[testPdev])
}

func TestGpuClientTrackerFdCount(t *testing.T) {
	proc := newFakeProc(t)
	proc.addProcess(100, "worker", "0::/system.slice/worker.service\n", []string{"/dev/null"}, []string{"pos: 0\n"})

	tracker, tick := newTestTracker(proc)
	counts := map[string]int64{"100": 1}
	tracker.fdCount = func(pid string) int64 { return counts[pid] }
	tracker.collect(60000)

	// with an unchanged fd count, fds aren't listed again on the fallback interval
	tick(gpuClientsStartupWindow)
	proc.addFd(100, 4, testRenderNode, i915Fdinfo(200, 0, 0))
	tick(gpuClientsFallbackInterval)
	assert.Empty(t, tracker.collect(60000))
	assert.Empty(t, tracker.procs["100"].fds, "fd count unchanged")

	// the full listing catches an fd swapped for another, and it only gets a baseline
	tick(gpuClientsRescanInterval - gpuClientsStartupWindow - gpuClientsFallbackInterval)
	proc.setFdinfo(100, 4, i915Fdinfo(200, 0, 5e9))
	assert.Empty(t, tracker.collect(60000))
	require.Equal(t, []string{"4"}, tracker.procs["100"].fds)
	tick(time.Second)
	proc.setFdinfo(100, 4, i915Fdinfo(200, 0, 5.1e9))
	assert.Equal(t, map[string]float64{"s:worker": 10}, tracker.collect(60000)[testPdev])

	// a changed count on a known process is listed once the backoff has passed
	proc.addFd(100, 5, testRenderNode, i915Fdinfo(201, 0, 0))
	counts["100"] = 2
	tick(gpuClientsRelistBackoff - 2*time.Second)
	tracker.collect(60000)
	assert.Equal(t, []string{"4"}, tracker.procs["100"].fds, "within the backoff")
	tick(time.Second)
	tracker.collect(60000)
	assert.ElementsMatch(t, []string{"4", "5"}, tracker.procs["100"].fds)

	// a young process is listed on every scan its count changes
	proc.addProcess(101, "ffmpeg", "0::/system.slice/ffmpeg.service\n", nil, nil)
	tick(time.Second)
	tracker.collect(60000)
	proc.addFd(101, 3, testRenderNode, i915Fdinfo(202, 0, 0))
	counts["101"] = 1
	tick(time.Second)
	tracker.collect(60000)
	assert.Equal(t, []string{"3"}, tracker.procs["101"].fds)
}

func TestListFdsIncremental(t *testing.T) {
	proc := newFakeProc(t)
	proc.addProcess(100, "worker", "0::/system.slice/worker.service\n", []string{"/dev/null", "/dev/null"}, []string{"pos: 0\n", "pos: 0\n"})
	tracker := &gpuClientTracker{procRoot: proc.root}
	drmProc := &drmProc{}

	require.NoError(t, tracker.listFds("100", drmProc, true))
	assert.Empty(t, drmProc.fds)
	assert.Len(t, drmProc.otherFds, 2)

	// known fds aren't resolved again, new ones are, and closed ones are dropped
	fd3 := filepath.Join(proc.root, "100", "fd", "3")
	require.NoError(t, os.Remove(fd3))
	require.NoError(t, os.Symlink(testRenderNode, fd3))
	proc.removeFd(100, 4)
	proc.addFd(100, 5, testRenderNode, i915Fdinfo(200, 0, 0))
	require.NoError(t, tracker.listFds("100", drmProc, false))
	assert.Equal(t, []string{"5"}, drmProc.fds)
	assert.Equal(t, map[string]struct{}{"3": {}}, drmProc.otherFds)

	// a full listing resolves every fd
	require.NoError(t, tracker.listFds("100", drmProc, true))
	assert.ElementsMatch(t, []string{"3", "5"}, drmProc.fds)
	assert.Empty(t, drmProc.otherFds)
}

func TestDrmUsageByConsumerCyclesAndCapacity(t *testing.T) {
	key := drmClientKey{device: "0000:03:00.0", id: 7}
	prev := &drmSnapshot{time: time.Unix(0, 0), clients: map[drmClientKey]drmClient{
		key: {consumer: "c:a", engines: map[string]drmEngine{
			"vcs": {cycles: 100, totalCycles: 1000, capacity: 2},
			"rcs": {cycles: 0, totalCycles: 1000, capacity: 1},
		}},
	}}
	cur := &drmSnapshot{time: time.Unix(1, 0), clients: map[drmClientKey]drmClient{
		key: {consumer: "c:a", engines: map[string]drmEngine{
			"vcs": {cycles: 900, totalCycles: 2000, capacity: 2}, // 800/1000 over 2 engines = 40%
			"rcs": {cycles: 250, totalCycles: 2000, capacity: 1}, // 25%
		}},
		// a new xe client has no total-cycles baseline and is skipped
		{device: "0000:03:00.0", id: 8}: {consumer: "c:b", engines: map[string]drmEngine{
			"rcs": {cycles: 500, totalCycles: 2000},
		}},
	}}
	assert.Equal(t, map[string]map[string]float64{"0000:03:00.0": {"c:a": 40}}, drmUsageByConsumer(prev, cur))
}

func TestGpuClientTrackerLateDiscovery(t *testing.T) {
	proc := newFakeProc(t)
	proc.addProcess(100, "jellyfin", dockerCgroup(testJellyfinId), []string{"/dev/null"}, []string{"pos: 0\n"})

	tracker, tick := newTestTracker(proc)
	tracker.collect(60000)
	tick(gpuClientsStartupWindow)
	tracker.collect(60000)

	// ffmpeg opens the GPU right after a scan and runs at 10% until its fds are listed again
	proc.addFd(100, 4, testRenderNode, i915Fdinfo(200, 0, 0))
	for i := range 5 {
		tick(time.Second)
		proc.setFdinfo(100, 4, i915Fdinfo(200, 0, uint64(i+1)*0.1e9))
		assert.Empty(t, tracker.collect(60000), "a known process's new client only gets a baseline")
	}
	require.Equal(t, []string{"4"}, tracker.procs["100"].fds)

	tick(time.Second)
	proc.setFdinfo(100, 4, i915Fdinfo(200, 0, 0.6e9))
	assert.Equal(t, map[string]float64{"c:" + testJellyfinId[:12]: 10}, tracker.collect(60000)[testPdev])
}

func TestGpuClientTrackerDiscoveryAcrossCacheTimes(t *testing.T) {
	proc := newFakeProc(t)
	proc.addProcess(100, "worker", "0::/system.slice/worker.service\n", []string{testRenderNode}, []string{i915Fdinfo(200, 0, 0)})

	tracker, tick := newTestTracker(proc)
	tracker.collect(60000)
	tracker.collect(1000)
	tick(gpuClientsStartupWindow)
	tracker.collect(1000)

	// a client found by the realtime scans is still new to the 60s snapshot,
	// but its process isn't, so it only gets a baseline there
	proc.addFd(100, 4, testRenderNode, i915Fdinfo(201, 0, 0))
	for i := range 5 {
		tick(time.Second)
		proc.setFdinfo(100, 4, i915Fdinfo(201, 0, uint64(i+1)*0.1e9))
		tracker.collect(1000)
	}
	tick(time.Second)
	proc.setFdinfo(100, 4, i915Fdinfo(201, 0, 0.6e9))
	assert.Equal(t, map[string]float64{"s:worker": 10}, tracker.collect(1000)[testPdev])
	assert.Empty(t, tracker.collect(60000))

	// a process started after the 60s snapshot starts from zero
	tick(time.Second)
	proc.addProcess(101, "ffmpeg", "0::/system.slice/ffmpeg.service\n", []string{testRenderNode}, []string{i915Fdinfo(202, 0, 0)})
	tracker.collect(1000)
	tick(time.Second)
	proc.setFdinfo(101, 3, i915Fdinfo(202, 0, 0.2e9))
	assert.Equal(t, map[string]float64{"s:ffmpeg": 10}, tracker.collect(60000)[testPdev])
}

// fakeSys creates a sysfs with a DRM card per PCI device bound to driver.
func fakeSys(t *testing.T, cards map[string][2]string) string {
	sys := t.TempDir()
	for card, dev := range cards {
		pdev, driver := dev[0], dev[1]
		pciDir := filepath.Join(sys, "bus/pci/devices", pdev)
		require.NoError(t, os.MkdirAll(filepath.Join(pciDir, "drm", card), 0o755))
		require.NoError(t, os.Symlink("../../../bus/pci/drivers/"+driver, filepath.Join(pciDir, "driver")))
		require.NoError(t, os.MkdirAll(filepath.Join(sys, "class/drm", card), 0o755))
		require.NoError(t, os.Symlink(pciDir, filepath.Join(sys, "class/drm", card, "device")))
	}
	return sys
}

func TestAttachGpuClients(t *testing.T) {
	const nvidiaPdev = "0000:01:00.0"
	intelUsage := func() map[string]map[string]float64 {
		return map[string]map[string]float64{testPdev: {"c:jellyfin": 30}}
	}
	intelAndNvidia := &gpuClientTracker{sysRoot: fakeSys(t, map[string][2]string{
		"card0": {nvidiaPdev, "nvidia"},
		"card1": {testPdev, "i915"},
	})}

	t.Run("matches sysfs card id", func(t *testing.T) {
		gpus := map[string]system.GPUData{"card1": {Name: "Intel"}, "card2": {Name: "AMD"}}
		intelAndNvidia.attachGpuClients(gpus, nil, intelUsage())
		assert.Equal(t, map[string]float64{"c:jellyfin": 30}, gpus["card1"].Clients)
		assert.Nil(t, gpus["card2"].Clients)
	})

	t.Run("intel_gpu_top and nvidia", func(t *testing.T) {
		gpus := map[string]system.GPUData{"i0": {}, "0": {}}
		vendors := map[string]string{"i0": gpuVendorIntel, "0": gpuVendorNvidia}
		intelAndNvidia.attachGpuClients(gpus, vendors, intelUsage())
		assert.Equal(t, map[string]float64{"c:jellyfin": 30}, gpus["i0"].Clients)
		assert.Nil(t, gpus["0"].Clients)
	})

	t.Run("intel clients are not put on an nvidia gpu", func(t *testing.T) {
		gpus := map[string]system.GPUData{"0": {}}
		intelAndNvidia.attachGpuClients(gpus, map[string]string{"0": gpuVendorNvidia}, intelUsage())
		assert.Nil(t, gpus["0"].Clients)
	})

	t.Run("gpu without a known vendor", func(t *testing.T) {
		gpus := map[string]system.GPUData{"n0": {}}
		intelAndNvidia.attachGpuClients(gpus, nil, intelUsage())
		assert.Nil(t, gpus["n0"].Clients)
	})

	t.Run("several cards of the vendor", func(t *testing.T) {
		tracker := &gpuClientTracker{sysRoot: fakeSys(t, map[string][2]string{
			"card0": {"0000:03:00.0", "i915"},
			"card1": {testPdev, "i915"},
		})}
		gpus := map[string]system.GPUData{"i0": {}}
		tracker.attachGpuClients(gpus, map[string]string{"i0": gpuVendorIntel}, intelUsage())
		assert.Nil(t, gpus["i0"].Clients, "intel_gpu_top may report the other card")
	})
}

func TestParseDrmFdinfo(t *testing.T) {
	t.Run("i915", func(t *testing.T) {
		key, engines, ok := parseDrmFdinfo([]byte(i915Fdinfo(179, 220198108, 189220148)))
		require.True(t, ok)
		assert.Equal(t, drmClientKey{device: testPdev, id: 179}, key)
		assert.Equal(t, uint64(220198108), engines["render"].busyNs)
		assert.Equal(t, uint64(189220148), engines["video"].busyNs)
		assert.Len(t, engines, 4)
	})

	t.Run("amdgpu with capacity", func(t *testing.T) {
		data := "drm-driver:\tamdgpu\ndrm-client-id:\t42\ndrm-pdev:\t0000:c4:00.0\ndrm-memory-vram:\t1024 KiB\n" +
			"drm-engine-gfx:\t1000 ns\ndrm-engine-dec:\t500 ns\ndrm-engine-capacity-dec:\t2\n"
		key, engines, ok := parseDrmFdinfo([]byte(data))
		require.True(t, ok)
		assert.Equal(t, drmClientKey{device: "0000:c4:00.0", id: 42}, key)
		assert.Equal(t, drmEngine{busyNs: 500, capacity: 2}, engines["dec"])
		assert.Equal(t, drmEngine{busyNs: 1000}, engines["gfx"])
	})

	t.Run("xe cycles", func(t *testing.T) {
		data := "drm-driver:\txe\ndrm-client-id:\t3\ndrm-pdev:\t0000:00:02.0\n" +
			"drm-cycles-vcs:\t123\ndrm-total-cycles-vcs:\t4567\ndrm-engine-capacity-vcs:\t2\n"
		_, engines, ok := parseDrmFdinfo([]byte(data))
		require.True(t, ok)
		assert.Equal(t, drmEngine{cycles: 123, totalCycles: 4567, capacity: 2}, engines["vcs"])
	})

	t.Run("missing pdev falls back to driver", func(t *testing.T) {
		key, _, ok := parseDrmFdinfo([]byte("drm-driver:\tv3d\ndrm-client-id:\t5\n"))
		require.True(t, ok)
		assert.Equal(t, drmClientKey{device: "v3d", id: 5}, key)
	})

	t.Run("not a drm client", func(t *testing.T) {
		_, _, ok := parseDrmFdinfo([]byte("pos:\t0\nflags:\t02\n"))
		assert.False(t, ok)
	})
}

func TestContainerIdFromCgroup(t *testing.T) {
	tests := map[string]string{
		"0::/../docker-" + testJellyfinId + ".scope\n":                              testJellyfinId,
		"0::/system.slice/docker-" + testJellyfinId + ".scope\n":                    testJellyfinId,
		"0::/docker/" + testJellyfinId + "\n":                                       testJellyfinId,
		"0::/machine.slice/libpod-" + testJellyfinId + ".scope/container\n":         testJellyfinId,
		"12:pids:/docker/" + testImmichId + "\n0::/docker/" + testJellyfinId + "\n": testJellyfinId,
		"0::/user.slice/user-1000.slice/session-569.scope\n":                        "",
		"0::/docker/" + testJellyfinId + "0\n":                                      "",
		"0::/kubepods/pod1234/" + testJellyfinId[:63] + "\n":                        "",
	}
	for cgroup, want := range tests {
		assert.Equal(t, want, containerIdFromCgroup([]byte(cgroup)), cgroup)
	}
}

func TestServiceFromCgroup(t *testing.T) {
	tests := map[string]string{
		"0::/system.slice/jellyfin.service\n":                                      "jellyfin",
		"0::/system.slice/system-getty.slice/getty@tty1.service\n":                 "getty@tty1",
		"1:name=systemd:/system.slice/old.service\n0::/system.slice/new.service\n": "new",
		"0::/user.slice/user-1000.slice/user@1000.service/app.slice/app-x.scope\n": "",
		"0::/user.slice/user-1000.slice/user@1000.service/app.slice/foo.service\n": "",
		"0::/user.slice/user-1000.slice/session-569.scope\n":                       "",
		"0::/init.scope\n": "",
	}
	for cgroup, want := range tests {
		assert.Equal(t, want, serviceFromCgroup([]byte(cgroup)), cgroup)
	}
}

func TestHasDrmClientStats(t *testing.T) {
	addCard := func(sys, card, driver string) {
		dir := filepath.Join(sys, "class/drm", card, "device")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.Symlink("../../../bus/pci/drivers/"+driver, filepath.Join(dir, "driver")))
	}

	none := &gpuClientTracker{sysRoot: t.TempDir()}
	assert.False(t, none.hasDrmClientStats())

	nvidia := &gpuClientTracker{sysRoot: t.TempDir()}
	addCard(nvidia.sysRoot, "card0", "nvidia")
	assert.False(t, nvidia.hasDrmClientStats())

	mixed := &gpuClientTracker{sysRoot: t.TempDir()}
	addCard(mixed.sysRoot, "card0", "nvidia")
	addCard(mixed.sysRoot, "card1", "i915")
	assert.True(t, mixed.hasDrmClientStats())
}
