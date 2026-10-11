//go:build testing

package agent

import (
	"bytes"
	"log/slog"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredMacVolumeAbsentAtStartup(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS mount-table registration")
	}
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	for _, usageErr := range []error{os.ErrNotExist, nil} {
		a := &Agent{fsStats: make(map[string]*system.FsStats)}
		d := diskDiscovery{
			agent: a,
			usageFn: func(string) (*disk.UsageStat, error) {
				// An unmounted volume can leave a directory on the root filesystem.
				return &disk.UsageStat{Total: 100}, usageErr
			},
		}
		d.addConfiguredExtraFsEntry("/Volumes/Backup", "backup")
		assert.Empty(t, a.fsStats)
	}
	assert.Empty(t, logs.String(), "an absent optional volume is not an error")
}

func TestConfiguredMacVolumeHotplug(t *testing.T) {
	root := &system.FsStats{Root: true, Mountpoint: "/", DiskTotal: 500}
	unconfigured := &system.FsStats{Mountpoint: "/Volumes/Other", DiskTotal: 200}
	rootBaseline := prevDisk{readBytes: 42, at: time.Now()}
	a := &Agent{
		fsStats:      map[string]*system.FsStats{"disk0": root, "disk9": unconfigured},
		fsNames:      []string{"disk0", "disk9"},
		diskBaseline: map[string]prevDisk{"disk0": rootBaseline},
		diskPrev: map[uint16]map[string]prevDisk{
			1000:  {"disk0": rootBaseline},
			60000: {"disk0": rootBaseline},
		},
	}
	d := diskDiscovery{agent: a, usageFn: func(path string) (*disk.UsageStat, error) {
		if path == "/" {
			return &disk.UsageStat{Total: 500 << 30}, nil
		}
		return &disk.UsageStat{Total: 100 << 30}, nil
	}, ctx: fsRegistrationContext{diskIoCounters: make(map[string]disk.IOCountersStat)}}
	const config = "/Volumes/Backup__backup"

	// The agent starts while the volume is absent.
	d.refreshMountedMacVolumes(config)
	require.Len(t, a.fsStats, 2)

	attach := func(device string, bytes uint64) {
		d.partitions = []disk.PartitionStat{{Device: "/dev/" + device, Mountpoint: "/Volumes/Backup"}}
		d.ctx.diskIoCounters = map[string]disk.IOCountersStat{device: {Name: device, ReadBytes: bytes, WriteBytes: bytes}}
		d.refreshMountedMacVolumes(config)
		require.Contains(t, a.fsStats, device)
		assert.Equal(t, "backup", a.fsStats[device].Name)
		assert.Equal(t, bytes, a.diskBaseline[device].readBytes)
		assert.Equal(t, bytes, a.fsStats[device].TotalRead)
	}
	attach("disk4", 1000)
	first := a.fsStats["disk4"]
	d.refreshMountedMacVolumes(config)
	assert.Same(t, first, a.fsStats["disk4"], "unchanged polls preserve counters and values")
	assert.Equal(t, []string{"disk0", "disk9", "disk4"}, a.fsNames)
	a.diskPrev[1000]["disk4"] = prevDisk{readBytes: 1000}
	a.diskPrev[60000]["disk4"] = prevDisk{readBytes: 1000}

	// Detaching must ignore a leftover directory even if usage still succeeds.
	d.partitions = nil
	d.refreshMountedMacVolumes(config)
	assert.NotContains(t, a.fsStats, "disk4")
	assert.NotContains(t, a.diskBaseline, "disk4")
	assert.NotContains(t, a.diskPrev[1000], "disk4")
	assert.NotContains(t, a.diskPrev[60000], "disk4")
	assert.NotContains(t, a.fsNames, "disk4")

	// Reattach with a new ID, then change ID between two collection calls.
	attach("disk6", 50)
	a.diskPrev[1000]["disk6"] = prevDisk{readBytes: 50}
	attach("disk4", 10)
	// A fast detach/reattach may reuse the same ID between collection calls.
	a.diskPrev[1000]["disk4"] = prevDisk{readBytes: 10}
	attach("disk4", 5)
	assert.NotContains(t, a.diskPrev[1000], "disk4")
	assert.NotContains(t, a.fsStats, "disk6")
	assert.NotContains(t, a.diskBaseline, "disk6")
	assert.NotContains(t, a.diskPrev[1000], "disk6")
	assert.NotContains(t, a.fsNames, "disk6")
	assert.Same(t, root, a.fsStats["disk0"])
	assert.Same(t, unconfigured, a.fsStats["disk9"])
	assert.Equal(t, rootBaseline, a.diskBaseline["disk0"])
	assert.Equal(t, rootBaseline, a.diskPrev[1000]["disk0"])
}

func TestMountedMacVolume(t *testing.T) {
	partitions := []disk.PartitionStat{{Mountpoint: "/Volumes"}, {Mountpoint: "/Volumes/Backup"}}
	for _, path := range []string{"/Volumes/Backup", "/Volumes/Backup/", "/Volumes/Backup/.beszel/disk4"} {
		assert.True(t, mountedMacVolume(path, partitions), path)
	}
	for _, path := range []string{"/Volumes", "/Volumes/Backups", "/Volumes/Missing"} {
		assert.False(t, mountedMacVolume(path, partitions), path)
	}
}

func TestConfiguredMacVolumeRealErrors(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })
	a := &Agent{fsStats: make(map[string]*system.FsStats)}
	d := diskDiscovery{agent: a,
		partitions: []disk.PartitionStat{{Mountpoint: "/Volumes/Backup"}},
		usageFn:    func(string) (*disk.UsageStat, error) { return nil, os.ErrPermission },
	}
	d.addConfiguredExtraFsEntry("/Volumes/Backup/private", "backup")
	assert.Empty(t, a.fsStats)
	assert.Contains(t, logs.String(), "level=ERROR")
	assert.Contains(t, logs.String(), "permission denied")
}

func TestMacVolumeDetachDuringUsageCollection(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS eject race")
	}
	t.Setenv("BESZEL_AGENT_EXTRA_FILESYSTEMS", "")
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })
	volume := &system.FsStats{Mountpoint: "/Volumes/BeszelMissingHotplugTest", DiskTotal: 100, DiskUsed: 50}
	a := &Agent{fsStats: map[string]*system.FsStats{"disk4": volume}}
	a.updateDiskUsage(&system.Stats{})
	assert.Zero(t, volume.DiskTotal)
	assert.Zero(t, volume.DiskUsed)
	assert.Empty(t, logs.String())
}

func TestMacVolumeDetachDuringUsageCache(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS mount-table refresh")
	}
	const missing = "/Volumes/BeszelMissingHotplugTest"
	t.Setenv("BESZEL_AGENT_EXTRA_FILESYSTEMS", missing)
	a := &Agent{
		fsStats:                map[string]*system.FsStats{"disk4": {Mountpoint: missing, DiskTotal: 100}},
		diskUsageCacheDuration: time.Hour,
		lastDiskUsageUpdate:    time.Now(),
	}
	a.updateDiskUsage(&system.Stats{})
	assert.Empty(t, a.fsStats, "cached usage must not keep a detached volume visible")
}

func TestMacVolumeHotplugRootDuplicate(t *testing.T) {
	root := &system.FsStats{Root: true, Mountpoint: "/"}
	a := &Agent{fsStats: map[string]*system.FsStats{"disk0": root}}
	d := diskDiscovery{agent: a,
		partitions: []disk.PartitionStat{{Device: "/dev/disk4", Mountpoint: "/Volumes/Backup"}},
		usageFn: func(string) (*disk.UsageStat, error) {
			return &disk.UsageStat{Total: 100 << 30, Used: 50 << 30}, nil
		},
	}
	d.refreshMountedMacVolumes("/Volumes/Backup")
	assert.Len(t, a.fsStats, 1)
	assert.Same(t, root, a.fsStats["disk0"])
	assert.Empty(t, a.fsNames)
}

func TestMacVolumeHotplugKeepsSubpathCached(t *testing.T) {
	stats := &system.FsStats{Mountpoint: "/Volumes/Backup/data", DiskTotal: 100}
	a := &Agent{fsStats: map[string]*system.FsStats{"data": stats}}
	d := diskDiscovery{agent: a,
		partitions: []disk.PartitionStat{{Device: "/dev/disk4", Mountpoint: "/Volumes/Backup"}},
		usageFn: func(string) (*disk.UsageStat, error) {
			t.Fatal("mount refresh must not query usage of an already registered subdirectory")
			return nil, nil
		},
	}
	d.refreshMountedMacVolumes("/Volumes/Backup/data")
	assert.Same(t, stats, a.fsStats["data"])
	assert.Equal(t, float64(100), stats.DiskTotal)
}
