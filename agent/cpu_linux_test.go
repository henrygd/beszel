//go:build testing && linux

package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// swapCpuContainerSeams points every container-detection and cgroup path at
// empty fixtures under a temp dir, then restores them on cleanup.
func swapCpuContainerSeams(t *testing.T) {
	t.Helper()
	backup := struct {
		root, mountinfo, selfCgroup, oneEnviron, dockerenv, containerenv, systemdCont string
		numCPU                                                                        func() int
		now                                                                           func() time.Time
	}{
		cpuCgroupRoot, cpuCgroupMountinfo, cpuProcSelfCgroup, cpuProcOneEnviron,
		cpuDockerenvPath, cpuContainerenv, cpuSystemdContPath, cpuNumCPU, cpuNow,
	}
	detected := containerDetected
	samples := lastCgroupCpuSamples
	env, hadEnv := os.LookupEnv("container")
	t.Cleanup(func() {
		cpuCgroupRoot, cpuCgroupMountinfo, cpuProcSelfCgroup, cpuProcOneEnviron = backup.root, backup.mountinfo, backup.selfCgroup, backup.oneEnviron
		cpuDockerenvPath, cpuContainerenv, cpuSystemdContPath = backup.dockerenv, backup.containerenv, backup.systemdCont
		cpuNumCPU, cpuNow = backup.numCPU, backup.now
		containerOnce = sync.Once{}
		containerDetected = detected
		lastCgroupCpuSamples = samples
		if hadEnv {
			os.Setenv("container", env)
		}
	})

	containerOnce = sync.Once{}
	containerDetected = false
	lastCgroupCpuSamples = make(map[uint16]cgroupCpuSample)
	os.Unsetenv("container")

	tmp := t.TempDir()
	cpuCgroupRoot = filepath.Join(tmp, "cgroup")
	cpuCgroupMountinfo = filepath.Join(tmp, "mountinfo")
	cpuProcSelfCgroup = filepath.Join(tmp, "self-cgroup")
	cpuProcOneEnviron = filepath.Join(tmp, "one-environ")
	cpuDockerenvPath = filepath.Join(tmp, "dockerenv")
	cpuContainerenv = filepath.Join(tmp, "containerenv")
	cpuSystemdContPath = filepath.Join(tmp, "systemd-container")
}

func writeCpuFixture(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

// fakeNow installs a controllable clock and returns a function to advance it.
func fakeNow(t *testing.T) func(time.Duration) {
	t.Helper()
	cur := time.Unix(1_700_000_000, 0)
	cpuNow = func() time.Time { return cur }
	return func(d time.Duration) { cur = cur.Add(d) }
}

func TestDetectContainer(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
		want  bool
	}{
		{"plain host", func(t *testing.T) {}, false},
		{"container env", func(t *testing.T) { t.Setenv("container", "lxc") }, true},
		{".dockerenv", func(t *testing.T) { writeCpuFixture(t, cpuDockerenvPath, "") }, true},
		{".containerenv", func(t *testing.T) { writeCpuFixture(t, cpuContainerenv, "") }, true},
		{"systemd container", func(t *testing.T) { writeCpuFixture(t, cpuSystemdContPath, "lxc\n") }, true},
		{"init environ container=lxc", func(t *testing.T) {
			writeCpuFixture(t, cpuProcOneEnviron, "PATH=/sbin\x00container=lxc\x00HOME=/root\x00")
		}, true},
		{"init environ without marker", func(t *testing.T) {
			writeCpuFixture(t, cpuProcOneEnviron, "PATH=/sbin\x00HOME=/root\x00")
		}, false},
		{"lxcfs serving /proc", func(t *testing.T) {
			writeCpuFixture(t, cpuCgroupMountinfo,
				"31 25 0:28 / /proc/stat rw,nosuid,nodev,relatime - fuse.lxcfs lxcfs rw,user_id=0,group_id=0\n")
		}, true},
		{"cgroup-only mountinfo", func(t *testing.T) {
			writeCpuFixture(t, cpuCgroupMountinfo,
				"36 25 0:32 / /sys/fs/cgroup rw - cgroup2 cgroup2 rw,nsdelegate\n")
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			swapCpuContainerSeams(t)
			tt.setup(t)
			assert.Equal(t, tt.want, detectContainer())
		})
	}
}

func TestReadCgroupV2CpuSample(t *testing.T) {
	swapCpuContainerSeams(t)
	writeCpuFixture(t, cpuProcSelfCgroup, "0::/\n")
	writeCpuFixture(t, cpuCgroupMountinfo, "")
	require.NoError(t, os.MkdirAll(cpuCgroupRoot, 0o755))
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"),
		"usage_usec 3000000\nuser_usec 2000000\nsystem_usec 1000000\nnr_throttled 7\n")
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpuset.cpus.effective"), "2,5-7\n")
	cpuNumCPU = func() int { return 8 }

	s, ok := readCgroupV2CpuSample()
	require.True(t, ok)
	assert.EqualValues(t, 3000000, s.usageUsec)
	assert.EqualValues(t, 2000000, s.userUsec)
	assert.EqualValues(t, 1000000, s.systemUsec)
	assert.InDelta(t, 4, s.cores, 0.001) // cpuset 2,5-7 = 4 cores
}

// In a namespaced container the agent may sit in a sub-cgroup (e.g. a systemd
// service); the mount root still accounts for the whole container and must win.
func TestReadCgroupV2PrefersContainerRoot(t *testing.T) {
	swapCpuContainerSeams(t)
	writeCpuFixture(t, cpuProcSelfCgroup, "0::/system.slice/beszel-agent.service\n")
	writeCpuFixture(t, cpuCgroupMountinfo, "")
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 9000\n")
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "system.slice/beszel-agent.service/cpu.stat"), "usage_usec 5\n")

	s, ok := readCgroupV2CpuSample()
	require.True(t, ok)
	assert.EqualValues(t, 9000, s.usageUsec)
}

// Without a cgroup namespace the mount shows the real host hierarchy and the
// container's own path (docker/kubepods/lxc markers) resolves under it.
func TestReadCgroupV2ResolvesRuntimePath(t *testing.T) {
	swapCpuContainerSeams(t)
	writeCpuFixture(t, cpuProcSelfCgroup, "0::/system.slice/docker-deadbeef.scope\n")
	writeCpuFixture(t, cpuCgroupMountinfo, "")
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 9000\n")
	sub := filepath.Join(cpuCgroupRoot, "system.slice/docker-deadbeef.scope")
	writeCpuFixture(t, filepath.Join(sub, "cpu.stat"), "usage_usec 5\n")
	writeCpuFixture(t, filepath.Join(sub, "cpu.max"), "100000 100000\n")

	s, ok := readCgroupV2CpuSample()
	require.True(t, ok)
	assert.EqualValues(t, 5, s.usageUsec)
	assert.InDelta(t, 1, s.cores, 0.001) // cpu.max quota of 1 core
}

func TestReadCgroupV1CpuSample(t *testing.T) {
	swapCpuContainerSeams(t)
	writeCpuFixture(t, cpuProcSelfCgroup, "3:cpuacct:/\n2:memory:/\n")
	v1 := filepath.Join(t.TempDir(), "cpuacct")
	writeCpuFixture(t, cpuCgroupMountinfo,
		"30 25 0:26 / "+v1+" rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,cpuacct\n")
	writeCpuFixture(t, filepath.Join(v1, "cpuacct.usage"), "2000000000\n")
	writeCpuFixture(t, filepath.Join(v1, "cpuacct.stat"), "user 100\nsystem 50\n")
	cpuNumCPU = func() int { return 4 }

	s, ok := readContainerCpuSample() // no 0:: line -> falls through to v1
	require.True(t, ok)
	assert.EqualValues(t, 2000000, s.usageUsec) // ns -> usec
	assert.EqualValues(t, 1000000, s.userUsec)  // 100 jiffies * 1e6/100
	assert.EqualValues(t, 500000, s.systemUsec) // 50 jiffies
	assert.InDelta(t, 4, s.cores, 0.001)
}

func TestContainerCpuMetricsMath(t *testing.T) {
	swapCpuContainerSeams(t)
	writeCpuFixture(t, cpuDockerenvPath, "")
	writeCpuFixture(t, cpuProcSelfCgroup, "0::/\n")
	writeCpuFixture(t, cpuCgroupMountinfo, "")
	require.NoError(t, os.MkdirAll(cpuCgroupRoot, 0o755))
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpuset.cpus.effective"), "0-3\n")
	cpuNumCPU = func() int { return 8 }
	advance := fakeNow(t)

	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"),
		"usage_usec 1000000\nuser_usec 600000\nsystem_usec 400000\n")
	m, ok := containerCpuMetrics(60000)
	require.True(t, ok)
	assert.Zero(t, m.Total) // first call only seeds the baseline

	// 1s elapsed, container burned 2 core-seconds on 4 usable cores
	advance(time.Second)
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"),
		"usage_usec 3000000\nuser_usec 1600000\nsystem_usec 900000\n")
	m, ok = containerCpuMetrics(60000)
	require.True(t, ok)
	assert.InDelta(t, 50, m.Total, 0.01)
	assert.InDelta(t, 25, m.User, 0.01)
	assert.InDelta(t, 12.5, m.System, 0.01)
	assert.Zero(t, m.Iowait)
	assert.Zero(t, m.Steal)
	assert.InDelta(t, 50, m.Idle, 0.01)
}

func TestContainerCpuMetricsHonorsQuota(t *testing.T) {
	swapCpuContainerSeams(t)
	writeCpuFixture(t, cpuDockerenvPath, "")
	writeCpuFixture(t, cpuProcSelfCgroup, "0::/\n")
	writeCpuFixture(t, cpuCgroupMountinfo, "")
	require.NoError(t, os.MkdirAll(cpuCgroupRoot, 0o755))
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.max"), "200000 100000\n") // 2 cores
	cpuNumCPU = func() int { return 8 }
	advance := fakeNow(t)

	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 1000000\n")
	containerCpuMetrics(60000)
	advance(time.Second)
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 2000000\n")
	m, ok := containerCpuMetrics(60000)
	require.True(t, ok)
	assert.InDelta(t, 50, m.Total, 0.01) // 1 core-second against a 2-core quota
}

func TestContainerCpuMetricsZeroAndBackwardDelta(t *testing.T) {
	swapCpuContainerSeams(t)
	writeCpuFixture(t, cpuDockerenvPath, "")
	writeCpuFixture(t, cpuProcSelfCgroup, "0::/\n")
	writeCpuFixture(t, cpuCgroupMountinfo, "")
	require.NoError(t, os.MkdirAll(cpuCgroupRoot, 0o755))
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 5000000\n")
	cpuNumCPU = func() int { return 4 }
	advance := fakeNow(t)

	// seed the baseline, then do not advance the clock: elapsed <= 0
	containerCpuMetrics(60000)
	m, ok := containerCpuMetrics(60000)
	require.True(t, ok)
	assert.Zero(t, m.Total)

	// counter goes backwards (cgroup recreated): report zero and re-baseline
	advance(time.Second)
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 100000\n")
	m, ok = containerCpuMetrics(60000)
	require.True(t, ok)
	assert.Zero(t, m.Total)

	// next tick measures from the new baseline, not the stale one
	advance(time.Second)
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 1100000\n")
	m, ok = containerCpuMetrics(60000)
	require.True(t, ok)
	assert.InDelta(t, 25, m.Total, 0.01) // 1e6 usec / (1s * 4 cores)
}

func TestContainerCpuMetricsFallbacks(t *testing.T) {
	t.Run("not in container", func(t *testing.T) {
		swapCpuContainerSeams(t)
		_, ok := containerCpuMetrics(60000)
		assert.False(t, ok)
	})
	t.Run("in container without cgroup accounting", func(t *testing.T) {
		swapCpuContainerSeams(t)
		writeCpuFixture(t, cpuDockerenvPath, "")
		writeCpuFixture(t, cpuProcSelfCgroup, "0::/\n")
		writeCpuFixture(t, cpuCgroupMountinfo, "")
		// cpuCgroupRoot has no cpu.stat
		_, ok := containerCpuMetrics(60000)
		assert.False(t, ok)
	})
}

// The host path must keep reporting through gopsutil untouched.
func TestGetCpuMetricsHostFallback(t *testing.T) {
	swapCpuContainerSeams(t)
	m, err := getCpuMetrics(60000)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, m.Total, 0.0)
	assert.LessOrEqual(t, m.Total, 100.0)
}

// Inside a container getCpuMetrics must report the cgroup-derived value, not
// the host core counters from /proc/stat.
func TestGetCpuMetricsPrefersCgroup(t *testing.T) {
	swapCpuContainerSeams(t)
	writeCpuFixture(t, cpuDockerenvPath, "")
	writeCpuFixture(t, cpuProcSelfCgroup, "0::/\n")
	writeCpuFixture(t, cpuCgroupMountinfo, "")
	require.NoError(t, os.MkdirAll(cpuCgroupRoot, 0o755))
	cpuNumCPU = func() int { return 4 }
	advance := fakeNow(t)

	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 0\n")
	m, err := getCpuMetrics(60000)
	require.NoError(t, err)
	assert.Zero(t, m.Total)

	advance(time.Second)
	writeCpuFixture(t, filepath.Join(cpuCgroupRoot, "cpu.stat"), "usage_usec 2000000\n")
	m, err = getCpuMetrics(60000)
	require.NoError(t, err)
	assert.InDelta(t, 50, m.Total, 0.01)
}

func TestCountCpuList(t *testing.T) {
	assert.Equal(t, 4, countCpuList("0-3"))
	assert.Equal(t, 4, countCpuList("2,5-7"))
	assert.Equal(t, 1, countCpuList("2"))
	assert.Equal(t, 0, countCpuList(""))
	assert.Equal(t, 0, countCpuList("max"))
	assert.Equal(t, 6, countCpuList("0-3,8-9"))
}

func TestCpuQuotaCores(t *testing.T) {
	dir := t.TempDir()
	_, ok := cpuQuotaCores(dir)
	assert.False(t, ok) // no quota files

	writeCpuFixture(t, filepath.Join(dir, "cpu.max"), "max 100000\n")
	_, ok = cpuQuotaCores(dir)
	assert.False(t, ok) // unlimited

	writeCpuFixture(t, filepath.Join(dir, "cpu.max"), "150000 100000\n")
	q, ok := cpuQuotaCores(dir)
	require.True(t, ok)
	assert.InDelta(t, 1.5, q, 0.001)

	// v1 files
	v1 := t.TempDir()
	writeCpuFixture(t, filepath.Join(v1, "cpu.cfs_quota_us"), "-1\n")
	writeCpuFixture(t, filepath.Join(v1, "cpu.cfs_period_us"), "100000\n")
	_, ok = cpuQuotaCores(v1)
	assert.False(t, ok)

	writeCpuFixture(t, filepath.Join(v1, "cpu.cfs_quota_us"), "50000\n")
	q, ok = cpuQuotaCores(v1)
	require.True(t, ok)
	assert.InDelta(t, 0.5, q, 0.001)
}
