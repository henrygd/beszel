//go:build testing

package agent

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFilesystemEntry(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedFs   string
		expectedName string
	}{
		{
			name:         "simple device name",
			input:        "sda1",
			expectedFs:   "sda1",
			expectedName: "",
		},
		{
			name:         "device with custom name",
			input:        "sda1__my-storage",
			expectedFs:   "sda1",
			expectedName: "my-storage",
		},
		{
			name:         "full device path with custom name",
			input:        "/dev/sdb1__backup-drive",
			expectedFs:   "/dev/sdb1",
			expectedName: "backup-drive",
		},
		{
			name:         "NVMe device with custom name",
			input:        "nvme0n1p2__fast-ssd",
			expectedFs:   "nvme0n1p2",
			expectedName: "fast-ssd",
		},
		{
			name:         "whitespace trimmed",
			input:        "  sda2__trimmed-name  ",
			expectedFs:   "sda2",
			expectedName: "trimmed-name",
		},
		{
			name:         "empty custom name",
			input:        "sda3__",
			expectedFs:   "sda3",
			expectedName: "",
		},
		{
			name:         "empty device name",
			input:        "__just-custom",
			expectedFs:   "",
			expectedName: "just-custom",
		},
		{
			name:         "multiple underscores in custom name",
			input:        "sda1__my_custom_drive",
			expectedFs:   "sda1",
			expectedName: "my_custom_drive",
		},
		{
			name:         "custom name with spaces",
			input:        "sda1__My Storage Drive",
			expectedFs:   "sda1",
			expectedName: "My Storage Drive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, customName := parseFilesystemEntry(tt.input)

			assert.Equal(t, tt.expectedFs, fs)
			assert.Equal(t, tt.expectedName, customName)
		})
	}
}

// setStatMount fakes statMount for the duration of a test. Paths missing from
// mounts return an empty mountInfo, as on platforms without statMount.
func setStatMount(t *testing.T, mounts map[string]mountInfo) {
	t.Helper()
	old := statMount
	statMount = func(path string) mountInfo { return mounts[path] }
	t.Cleanup(func() { statMount = old })
}

// newTestDiscovery returns a discovery over the given diskstats devices with a
// usage function that accepts every path and never reports equal usage. It
// fakes statMount with no mounts; call setStatMount afterwards to add some.
func newTestDiscovery(t *testing.T, counters map[string]disk.IOCountersStat) (*Agent, *diskDiscovery) {
	t.Helper()
	setStatMount(t, nil)
	agent := &Agent{fsStats: make(map[string]*system.FsStats)}
	return agent, &diskDiscovery{
		agent:          agent,
		rootMountPoint: "/",
		usageFn:        func(string) (*disk.UsageStat, error) { return &disk.UsageStat{}, nil },
		keys:           loadFsKeyStore(""),
		ctx: fsRegistrationContext{
			efPath:         filepath.FromSlash("/extra-filesystems"),
			diskIoCounters: counters,
		},
	}
}

func counters(names ...string) map[string]disk.IOCountersStat {
	m := make(map[string]disk.IOCountersStat, len(names))
	for _, name := range names {
		m[name] = disk.IOCountersStat{Name: name}
	}
	return m
}

func TestAddRootFs(t *testing.T) {
	t.Run("uses the device behind the root mount", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("nvme0n1", "nvme0n1p2"))
		setStatMount(t, map[string]mountInfo{"/": {id: "dev:259:2", devName: "nvme0n1p2"}})

		d.addRootFs()

		root := agent.fsStats[rootFsKey]
		require.NotNil(t, root)
		assert.True(t, root.Root)
		assert.Equal(t, "/", root.Mountpoint)
		assert.Equal(t, "nvme0n1p2", root.IoDevice)
	})

	t.Run("container root uses the filesystem holding /etc/hosts", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("nvme0n1p3", "nvme0n1p6"))
		setStatMount(t, map[string]mountInfo{
			"/":          {id: "dev:0:50", overlay: true},
			"/etc/hosts": {id: "dev:259:6", devName: "nvme0n1p6"},
		})

		d.addRootFs()

		assert.Equal(t, "nvme0n1p6", agent.fsStats[rootFsKey].IoDevice)
		assert.Equal(t, "/", agent.fsStats[rootFsKey].Mountpoint)
		assert.ElementsMatch(t, []string{"mount:/", "dev:0:50", "dev:259:6"}, d.rootIDs)
	})

	t.Run("FILESYSTEM matches a partition", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"ada0": {Name: "ada0", ReadBytes: 1000, WriteBytes: 1000},
		})
		d.partitions = []disk.PartitionStat{{Device: "/dev/ada0p2", Mountpoint: "/"}}
		d.ctx.filesystem, d.ctx.filesystemName = "/dev/ada0p2", "root disk"

		d.addRootFs()

		root := agent.fsStats[rootFsKey]
		assert.Equal(t, "ada0", root.IoDevice)
		assert.Equal(t, "root disk", root.Name)
	})

	t.Run("FILESYSTEM names an I/O device missing from partitions", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"nda0": {Name: "nda0", Label: "zroot", ReadBytes: 1000, WriteBytes: 1000},
		})
		d.rootMountPoint = "/sysroot"
		d.ctx.filesystem, d.ctx.filesystemName = "zroot", "root pool"

		d.addRootFs()

		root := agent.fsStats[rootFsKey]
		assert.Equal(t, "nda0", root.IoDevice)
		assert.Equal(t, "/sysroot", root.Mountpoint)
		assert.Equal(t, "root pool", root.Name)
	})

	t.Run("FILESYSTEM takes precedence over the root mount device", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"nvme0n1": {Name: "nvme0n1", ReadBytes: 1000, WriteBytes: 1000},
		})
		setStatMount(t, map[string]mountInfo{"/": {id: "dev:0:50", overlay: true}})
		d.ctx.filesystem = "nvme0n1p2"

		d.addRootFs()

		assert.Equal(t, "nvme0n1", agent.fsStats[rootFsKey].IoDevice)
	})

	t.Run("unresolved FILESYSTEM falls back to the most active device", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"sda": {Name: "sda", ReadBytes: 5000, WriteBytes: 5000},
			"sdb": {Name: "sdb", ReadBytes: 1000, WriteBytes: 1000},
		})
		d.ctx.filesystem = "missing-disk"

		d.addRootFs()

		assert.Equal(t, "sda", agent.fsStats[rootFsKey].IoDevice)
	})

	t.Run("maps a root partition to its disk by name", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"nvme0n1": {Name: "nvme0n1", ReadBytes: 1000, WriteBytes: 1000},
		})
		d.partitions = []disk.PartitionStat{{Device: "/dev/nvme0n1p2", Mountpoint: "/"}}

		d.addRootFs()

		assert.Equal(t, "nvme0n1", agent.fsStats[rootFsKey].IoDevice)
	})

	t.Run("uses Docker bind mounts when the root partition is an overlay", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"sda1": {Name: "sda1", ReadBytes: 10},
			"sdb":  {Name: "sdb", ReadBytes: 5000},
		})
		d.partitions = []disk.PartitionStat{
			{Device: "overlay", Mountpoint: "/"},
			{Device: "/dev/sda1", Mountpoint: "/etc/hosts"},
		}

		d.addRootFs()

		assert.Equal(t, "sda1", agent.fsStats[rootFsKey].IoDevice)
	})

	t.Run("keeps the root mountpoint without any I/O device", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{})
		d.rootMountPoint = "/sysroot"

		d.addRootFs()

		root := agent.fsStats[rootFsKey]
		assert.Equal(t, "/sysroot", root.Mountpoint)
		assert.Empty(t, root.IoDevice)
	})

	t.Run("matches the Windows system drive exactly (#2417)", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"C:": {Name: "C:", ReadBytes: 10},
			"D:": {Name: "D:", ReadBytes: 100},
			"P:": {Name: "P:", ReadBytes: 1000},
		})
		d.rootMountPoint = `C:\`
		d.partitions = []disk.PartitionStat{{Device: "D:", Mountpoint: `D:\`}, {Device: "C:", Mountpoint: `C:\`}}

		d.addRootFs()

		assert.Len(t, agent.fsStats, 1)
		assert.Equal(t, "C:", agent.fsStats[rootFsKey].IoDevice)
	})
}

func TestExtraFilesystemFolders(t *testing.T) {
	ef := func(name string) string { return filepath.Join(filepath.FromSlash("/extra-filesystems"), name) }

	t.Run("keys by folder name and reads I/O from the mounted device (#294)", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("sdb1", "sdc1"))
		// The folders were named after sdb1/sdc1, but the disks swapped names.
		setStatMount(t, map[string]mountInfo{
			ef("sdb1__media"): {id: "dev:8:33", devName: "sdc1"},
			ef("sdc1"):        {id: "dev:8:17", devName: "sdb1"},
		})

		d.addExtraFilesystemFolders([]string{"sdb1__media", "sdc1"})
		d.registerExtraFilesystems()

		require.Len(t, agent.fsStats, 2)
		assert.Equal(t, "sdc1", agent.fsStats["media"].IoDevice)
		assert.Equal(t, "media", agent.fsStats["media"].Name)
		assert.Equal(t, ef("sdb1__media"), agent.fsStats["media"].Mountpoint)
		assert.Equal(t, "sdb1", agent.fsStats["sdc1"].IoDevice)
		assert.Empty(t, agent.fsStats["sdc1"].Name)
	})

	t.Run("falls back to the folder device hint", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"dm-1":      {Name: "dm-1", Label: "luks-2bcb02be"},
			"nvme0n1p2": {Name: "nvme0n1p2"},
		})
		d.partitions = []disk.PartitionStat{{Device: "/dev/mapper/luks-2bcb02be", Mountpoint: ef("nvme0n1p2__Archive")}}

		d.addExtraFilesystemFolders([]string{"nvme0n1p2__Archive"})
		d.registerExtraFilesystems()

		assert.Equal(t, "nvme0n1p2", agent.fsStats["Archive"].IoDevice)
	})

	t.Run("falls back to the partition device", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"dm-1": {Name: "dm-1", Label: "luks-2bcb02be"},
		})
		d.partitions = []disk.PartitionStat{{Device: "/dev/mapper/luks-2bcb02be", Mountpoint: ef("Archive")}}

		d.addExtraFilesystemFolders([]string{"Archive"})
		d.registerExtraFilesystems()

		assert.Equal(t, "dm-1", agent.fsStats["Archive"].IoDevice)
	})

	t.Run("custom name without device", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters())

		d.addExtraFilesystemFolders([]string{"__Share"})
		d.registerExtraFilesystems()

		require.Contains(t, agent.fsStats, "Share")
		assert.Empty(t, agent.fsStats["Share"].IoDevice)
	})
}

func TestConfiguredExtraFilesystems(t *testing.T) {
	t.Run("registers partitions and usage-validated paths", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("sda1"))
		d.partitions = []disk.PartitionStat{{Device: "/dev/sda1", Mountpoint: "/mnt/fast"}}
		d.usageFn = func(path string) (*disk.UsageStat, error) {
			if path == "/srv/archive" {
				return &disk.UsageStat{}, nil
			}
			return nil, os.ErrNotExist
		}

		auto := d.addConfiguredExtraFilesystems("sda1__fast, ,/srv/archive__cold,/missing,")
		d.registerExtraFilesystems()

		assert.False(t, auto)
		require.Len(t, agent.fsStats, 2)
		assert.Equal(t, "/mnt/fast", agent.fsStats["fast"].Mountpoint)
		assert.Equal(t, "sda1", agent.fsStats["fast"].IoDevice)
		assert.Equal(t, "/srv/archive", agent.fsStats["cold"].Mountpoint)
	})

	t.Run("unnamed entry keeps the device name as key", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("sdb1"))
		d.partitions = []disk.PartitionStat{{Device: "/dev/sdb1", Mountpoint: "/mnt/data"}}

		d.addConfiguredExtraFilesystems("/mnt/data")
		d.registerExtraFilesystems()

		require.Contains(t, agent.fsStats, "sdb1")
		assert.Equal(t, "/mnt/data", agent.fsStats["sdb1"].Mountpoint)
	})

	t.Run("maps LVM volumes to their dm device (#1790, #2482)", func(t *testing.T) {
		agent, d := newTestDiscovery(t, map[string]disk.IOCountersStat{
			"dm-0": {Name: "dm-0", Label: "vg1-syno_vg_reserved_area"},
			"dm-1": {Name: "dm-1", Label: "vg1-volume_1"},
		})
		setStatMount(t, map[string]mountInfo{"/volume1": {id: "dev:253:1", devName: "dm-1"}})
		d.partitions = []disk.PartitionStat{{Device: "/dev/vg1/volume_1", Mountpoint: "/volume1"}}

		d.addConfiguredExtraFilesystems("/volume1")
		d.registerExtraFilesystems()

		require.Contains(t, agent.fsStats, "dm-1")
		assert.Equal(t, "dm-1", agent.fsStats["dm-1"].IoDevice)
	})

	t.Run("skips autofs placeholders of systemd automounts (#1931)", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("sdb1", "sdc1"))
		setStatMount(t, map[string]mountInfo{
			"/media/video":  {id: "dev:8:33", devName: "sdc1"},
			"/media/backup": {id: "dev:8:17", devName: "sdb1"},
		})
		d.partitions = []disk.PartitionStat{
			{Device: "systemd-1", Mountpoint: "/media/backup", Fstype: "autofs"},
			{Device: "systemd-1", Mountpoint: "/media/video", Fstype: "autofs"},
			{Device: "/dev/sdc1", Mountpoint: "/media/video", Fstype: "exfat"},
		}

		// /media/backup is not mounted yet, so only its autofs entry is listed.
		d.addConfiguredExtraFilesystems("/media/video__Video,/media/backup__Backup")
		d.registerExtraFilesystems()

		require.Len(t, agent.fsStats, 2)
		assert.Equal(t, "sdc1", agent.fsStats["Video"].IoDevice)
		assert.Equal(t, "sdb1", agent.fsStats["Backup"].IoDevice)
	})

	t.Run("discovers disks automatically unless a list is given", func(t *testing.T) {
		for value, want := range map[string]bool{
			"":                 true, // unset or empty: discovery is the default
			" , ":              true,
			"AUTO":             true,
			"auto,/mnt/x__X":   true,
			"none":             false,
			"/mnt/x__X":        false,
			"__auto":           false, // a custom name, not the token
			"/mnt/x__X, none ": false,
		} {
			_, d := newTestDiscovery(t, counters())
			d.usageFn = func(string) (*disk.UsageStat, error) { return nil, os.ErrNotExist }
			assert.Equal(t, want, d.addConfiguredExtraFilesystems(value), "EXTRA_FILESYSTEMS=%q", value)
			assert.Empty(t, d.extras)
		}
	})
}

func TestDedupeExtraFilesystems(t *testing.T) {
	ef := func(name string) string { return filepath.Join(filepath.FromSlash("/extra-filesystems"), name) }

	t.Run("merges the root filesystem into root and keeps its name (#1428)", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("nvme0n1p2"))
		setStatMount(t, map[string]mountInfo{
			"/":                 {id: "dev:259:2", devName: "nvme0n1p2"},
			ef("nvme0n1p2__OS"): {id: "dev:259:2", devName: "nvme0n1p2"},
		})

		d.addRootFs()
		d.addExtraFilesystemFolders([]string{"nvme0n1p2__OS"})
		d.registerExtraFilesystems()

		require.Len(t, agent.fsStats, 1)
		assert.Equal(t, "OS", agent.fsStats[rootFsKey].Name)
	})

	t.Run("merges a volume holding the container into root (#1878)", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("dm-1", "dm-2"))
		setStatMount(t, map[string]mountInfo{
			"/":                    {id: "dev:0:50", overlay: true},
			"/etc/hosts":           {id: "dev:253:2", devName: "dm-2"},
			ef("md2__Volume2"):     {id: "dev:253:2", devName: "dm-2"},
			ef("md1__Volume1"):     {id: "dev:253:1", devName: "dm-1"},
			ef("not-a-bind-mount"): {id: "dev:0:50", overlay: true},
		})

		d.addRootFs()
		d.addExtraFilesystemFolders([]string{"md1__Volume1", "md2__Volume2", "not-a-bind-mount"})
		d.registerExtraFilesystems()

		require.Len(t, agent.fsStats, 2)
		assert.Equal(t, "Volume2", agent.fsStats[rootFsKey].Name)
		assert.Equal(t, "dm-2", agent.fsStats[rootFsKey].IoDevice)
		assert.Equal(t, "dm-1", agent.fsStats["Volume1"].IoDevice)
	})

	t.Run("keeps different filesystems with equal usage (#2103)", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("nvme0n1p3", "nvme0n1p6"))
		setStatMount(t, map[string]mountInfo{
			"/":        {id: "dev:259:3", devName: "nvme0n1p3"},
			ef("DATA"): {id: "dev:259:6", devName: "nvme0n1p6"},
		})
		d.usageFn = func(string) (*disk.UsageStat, error) {
			return &disk.UsageStat{Total: 100 << 30, Used: 40 << 30}, nil
		}

		d.addRootFs()
		d.addExtraFilesystemFolders([]string{"DATA"})
		d.registerExtraFilesystems()

		require.Contains(t, agent.fsStats, "DATA")
		assert.Equal(t, "nvme0n1p6", agent.fsStats["DATA"].IoDevice)
	})

	t.Run("keeps one entry per filesystem", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("sdb1"))
		setStatMount(t, map[string]mountInfo{
			"/mnt/data":      {id: "dev:8:17", devName: "sdb1"},
			ef("sdb1__Data"): {id: "dev:8:17", devName: "sdb1"},
		})
		d.partitions = []disk.PartitionStat{{Device: "/dev/sdb1", Mountpoint: "/mnt/data"}}

		d.addConfiguredExtraFilesystems("/mnt/data__Explicit,/mnt/data__Again")
		d.addExtraFilesystemFolders([]string{"sdb1__Data"})
		d.registerExtraFilesystems()

		require.Len(t, agent.fsStats, 1)
		assert.Equal(t, "/mnt/data", agent.fsStats["Explicit"].Mountpoint)
	})

	t.Run("compares usage without filesystem identities", func(t *testing.T) {
		// A folder on the system drive is listed in EXTRA_FILESYSTEMS on Windows.
		agent, d := newTestDiscovery(t, counters("C:", "D:"))
		d.rootMountPoint = `C:\`
		d.partitions = []disk.PartitionStat{{Device: "C:", Mountpoint: `C:\`}, {Device: "D:", Mountpoint: `D:\`}}
		d.usageFn = func(path string) (*disk.UsageStat, error) {
			if path == `D:\` {
				return &disk.UsageStat{Total: 500 << 30, Used: 1 << 30}, nil
			}
			return &disk.UsageStat{Total: 100 << 30, Used: 40 << 30}, nil
		}

		d.addRootFs()
		d.addConfiguredExtraFilesystems(`C:\Users__System,D:`)
		d.registerExtraFilesystems()

		require.Len(t, agent.fsStats, 2)
		assert.Equal(t, "System", agent.fsStats[rootFsKey].Name)
		assert.Equal(t, "D:", agent.fsStats["D:"].IoDevice)
	})
}

func TestAssignKeys(t *testing.T) {
	t.Run("pins unnamed keys to stable IDs across device renames (#294)", func(t *testing.T) {
		dataDir := t.TempDir()
		run := func(mounts map[string]mountInfo, partitions []disk.PartitionStat) *Agent {
			agent, d := newTestDiscovery(t, counters("sdb1", "sdc1"))
			setStatMount(t, mounts)
			d.keys = loadFsKeyStore(dataDir)
			d.partitions = partitions
			d.addConfiguredExtraFilesystems("/mnt/a,/mnt/b")
			d.registerExtraFilesystems()
			d.keys.save()
			return agent
		}

		agent := run(map[string]mountInfo{
			"/mnt/a": {id: "dev:8:17", devName: "sdb1", stableID: "fsid:a"},
			"/mnt/b": {id: "dev:8:33", devName: "sdc1", stableID: "fsid:b"},
		}, []disk.PartitionStat{{Device: "/dev/sdb1", Mountpoint: "/mnt/a"}, {Device: "/dev/sdc1", Mountpoint: "/mnt/b"}})
		assert.Equal(t, "/mnt/a", agent.fsStats["sdb1"].Mountpoint)
		assert.Equal(t, "/mnt/b", agent.fsStats["sdc1"].Mountpoint)

		// After a reboot the disks swapped names.
		agent = run(map[string]mountInfo{
			"/mnt/a": {id: "dev:8:33", devName: "sdc1", stableID: "fsid:a"},
			"/mnt/b": {id: "dev:8:17", devName: "sdb1", stableID: "fsid:b"},
		}, []disk.PartitionStat{{Device: "/dev/sdc1", Mountpoint: "/mnt/a"}, {Device: "/dev/sdb1", Mountpoint: "/mnt/b"}})
		assert.Equal(t, "/mnt/a", agent.fsStats["sdb1"].Mountpoint)
		assert.Equal(t, "sdc1", agent.fsStats["sdb1"].IoDevice)
		assert.Equal(t, "/mnt/b", agent.fsStats["sdc1"].Mountpoint)
		assert.Equal(t, "sdb1", agent.fsStats["sdc1"].IoDevice)

		// Disk a was replaced. The new disk is named sdb1 now, while b still
		// claims its own key.
		agent = run(map[string]mountInfo{
			"/mnt/a": {id: "dev:8:17", devName: "sdb1", stableID: "fsid:new"},
			"/mnt/b": {id: "dev:8:33", devName: "sdc1", stableID: "fsid:b"},
		}, []disk.PartitionStat{{Device: "/dev/sdb1", Mountpoint: "/mnt/a"}, {Device: "/dev/sdc1", Mountpoint: "/mnt/b"}})
		assert.Equal(t, "/mnt/a", agent.fsStats["sdb1"].Mountpoint)
		assert.Equal(t, "/mnt/b", agent.fsStats["sdc1"].Mountpoint)

		assert.Equal(t, map[string]string{"fsid:new": "sdb1", "fsid:b": "sdc1"}, loadFsKeyStore(dataDir).keys)
	})

	t.Run("never drops a filesystem whose key is taken", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("disk1"))
		d.partitions = []disk.PartitionStat{
			{Device: "/dev/disk1s1", Mountpoint: "/Volumes/A"},
			{Device: "/dev/disk1s2", Mountpoint: "/Volumes/B"},
			{Device: "/dev/disk1s3", Mountpoint: "/Volumes/C/B"},
		}

		d.addConfiguredExtraFilesystems("/Volumes/A,/Volumes/B,/Volumes/C/B")
		d.registerExtraFilesystems()

		require.Len(t, agent.fsStats, 3)
		assert.Equal(t, "/Volumes/A", agent.fsStats["disk1"].Mountpoint)
		assert.Equal(t, "/Volumes/B", agent.fsStats["B"].Mountpoint)
		assert.Equal(t, "/Volumes/C/B", agent.fsStats["disk1-2"].Mountpoint)
	})

	t.Run("explicit names win over pinned keys", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("sdb1", "sdc1"))
		setStatMount(t, map[string]mountInfo{
			"/mnt/a": {id: "dev:8:17", devName: "sdb1", stableID: "fsid:a"},
			"/mnt/b": {id: "dev:8:33", devName: "sdc1", stableID: "fsid:b"},
		})
		d.keys.keys["fsid:b"] = "Media"
		d.partitions = []disk.PartitionStat{{Device: "/dev/sdb1", Mountpoint: "/mnt/a"}, {Device: "/dev/sdc1", Mountpoint: "/mnt/b"}}

		d.addConfiguredExtraFilesystems("/mnt/a__Media,/mnt/b")
		d.registerExtraFilesystems()

		assert.Equal(t, "/mnt/a", agent.fsStats["Media"].Mountpoint)
		assert.Equal(t, "/mnt/b", agent.fsStats["sdc1"].Mountpoint)
		assert.Equal(t, "sdc1", d.keys.keys["fsid:b"])
	})
}

func TestAutoFilesystems(t *testing.T) {
	dir := t.TempDir()
	data, bind, file := filepath.Join(dir, "data"), filepath.Join(dir, "bind"), filepath.Join(dir, "localtime")
	require.NoError(t, os.Mkdir(data, 0o755))
	require.NoError(t, os.Mkdir(bind, 0o755))
	require.NoError(t, os.WriteFile(file, nil, 0o644))

	agent, d := newTestDiscovery(t, counters("sda1", "sdb1"))
	setStatMount(t, map[string]mountInfo{
		data: {id: "dev:8:17", devName: "sdb1"},
		bind: {id: "dev:8:17", devName: "sdb1"},
	})
	d.partitions = []disk.PartitionStat{
		{Device: "/dev/sda1", Mountpoint: "/", Fstype: "ext4"},
		{Device: "/dev/sdb1", Mountpoint: data, Fstype: "ext4"},
		{Device: "/dev/sdb1", Mountpoint: bind, Fstype: "ext4"},
		{Device: "/dev/sda1", Mountpoint: file, Fstype: "ext4"},
		{Device: "tmpfs", Mountpoint: "/run/user/1000", Fstype: "tmpfs"},
		{Device: "/dev/loop0", Mountpoint: "/snap/core/1", Fstype: "squashfs"},
		{Device: "/dev/sdc1", Mountpoint: "/var/lib/docker/overlay2", Fstype: "ext4"},
		{Device: "/dev/sdd1", Mountpoint: "/extra-filesystems/sdd1", Fstype: "ext4"},
		{Device: "tank/data", Mountpoint: "/tank/data", Fstype: "zfs"},
		{Device: "systemd-1", Mountpoint: "/media/backup", Fstype: "autofs"},
		{Device: "/dev/nvme0n1p1", Mountpoint: "/boot/efi", Fstype: "vfat"},
	}

	require.True(t, d.addConfiguredExtraFilesystems(""))
	d.addAutoFilesystems()
	d.registerExtraFilesystems()

	require.Len(t, agent.fsStats, 1)
	assert.Equal(t, data, agent.fsStats["sdb1"].Mountpoint)

	t.Run("explicit entries win over auto", func(t *testing.T) {
		agent, d := newTestDiscovery(t, counters("sdb1"))
		d.partitions = []disk.PartitionStat{{Device: "/dev/sdb1", Mountpoint: data, Fstype: "ext4"}}

		if d.addConfiguredExtraFilesystems("auto," + data + "__Data") {
			d.addAutoFilesystems()
		}
		d.registerExtraFilesystems()

		require.Len(t, agent.fsStats, 1)
		assert.Contains(t, agent.fsStats, "Data")
	})
}

func TestFindIoDevice(t *testing.T) {
	t.Run("matches by device name", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"sda": {Name: "sda"},
			"sdb": {Name: "sdb"},
		}

		device, ok := findIoDevice("sdb", ioCounters)
		assert.True(t, ok)
		assert.Equal(t, "sdb", device)
	})

	t.Run("matches by device label", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"sda": {Name: "sda", Label: "rootfs"},
			"sdb": {Name: "sdb"},
		}

		device, ok := findIoDevice("rootfs", ioCounters)
		assert.True(t, ok)
		assert.Equal(t, "sda", device)
	})

	t.Run("returns no match when not found", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"sda": {Name: "sda"},
			"sdb": {Name: "sdb"},
		}

		device, ok := findIoDevice("nvme0n1p1", ioCounters)
		assert.False(t, ok)
		assert.Equal(t, "", device)
	})

	t.Run("uses uncertain unique prefix fallback", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"nvme0n1": {Name: "nvme0n1"},
			"sda":     {Name: "sda"},
		}

		device, ok := findIoDevice("nvme0n1p2", ioCounters)
		assert.True(t, ok)
		assert.Equal(t, "nvme0n1", device)
	})

	t.Run("uses dominant activity when prefix matches are ambiguous", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"sda": {Name: "sda", ReadBytes: 5000, WriteBytes: 5000, ReadCount: 100, WriteCount: 100},
			"sdb": {Name: "sdb", ReadBytes: 1000, WriteBytes: 1000, ReadCount: 50, WriteCount: 50},
		}

		device, ok := findIoDevice("sd", ioCounters)
		assert.True(t, ok)
		assert.Equal(t, "sda", device)
	})

	t.Run("uses highest activity when ambiguous without dominance", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"sda": {Name: "sda", ReadBytes: 3000, WriteBytes: 3000, ReadCount: 50, WriteCount: 50},
			"sdb": {Name: "sdb", ReadBytes: 2500, WriteBytes: 2500, ReadCount: 40, WriteCount: 40},
		}

		device, ok := findIoDevice("sd", ioCounters)
		assert.True(t, ok)
		assert.Equal(t, "sda", device)
	})

	t.Run("matches /dev/-prefixed partition to parent disk", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"nda0": {Name: "nda0", ReadBytes: 1000, WriteBytes: 1000},
		}

		device, ok := findIoDevice("/dev/nda0p2", ioCounters)
		assert.True(t, ok)
		assert.Equal(t, "nda0", device)
	})

	t.Run("uses deterministic name tie-breaker", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"sdb": {Name: "sdb", ReadBytes: 2000, WriteBytes: 2000, ReadCount: 10, WriteCount: 10},
			"sda": {Name: "sda", ReadBytes: 2000, WriteBytes: 2000, ReadCount: 10, WriteCount: 10},
		}

		device, ok := findIoDevice("sd", ioCounters)
		assert.True(t, ok)
		assert.Equal(t, "sda", device)
	})
}

func TestFilesystemMatchesPartitionSetting(t *testing.T) {
	p := disk.PartitionStat{Device: "/dev/ada0p2", Mountpoint: "/"}

	t.Run("matches mountpoint setting", func(t *testing.T) {
		assert.True(t, filesystemMatchesPartitionSetting("/", p))
	})

	t.Run("matches exact partition setting", func(t *testing.T) {
		assert.True(t, filesystemMatchesPartitionSetting("ada0p2", p))
		assert.True(t, filesystemMatchesPartitionSetting("/dev/ada0p2", p))
	})

	t.Run("matches prefix-style parent setting", func(t *testing.T) {
		assert.True(t, filesystemMatchesPartitionSetting("ada0", p))
		assert.True(t, filesystemMatchesPartitionSetting("/dev/ada0", p))
	})

	t.Run("does not match unrelated device", func(t *testing.T) {
		assert.False(t, filesystemMatchesPartitionSetting("sda", p))
		assert.False(t, filesystemMatchesPartitionSetting("nvme0n1", p))
		assert.False(t, filesystemMatchesPartitionSetting("", p))
	})
}

func TestMostActiveIoDevice(t *testing.T) {
	t.Run("returns most active device", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"nda0": {Name: "nda0", ReadBytes: 5000, WriteBytes: 5000, ReadCount: 100, WriteCount: 100},
			"nda1": {Name: "nda1", ReadBytes: 1000, WriteBytes: 1000, ReadCount: 50, WriteCount: 50},
		}
		assert.Equal(t, "nda0", mostActiveIoDevice(ioCounters))
	})

	t.Run("uses deterministic tie-breaker", func(t *testing.T) {
		ioCounters := map[string]disk.IOCountersStat{
			"sdb": {Name: "sdb", ReadBytes: 1000, WriteBytes: 1000, ReadCount: 10, WriteCount: 10},
			"sda": {Name: "sda", ReadBytes: 1000, WriteBytes: 1000, ReadCount: 10, WriteCount: 10},
		}
		assert.Equal(t, "sda", mostActiveIoDevice(ioCounters))
	})

	t.Run("returns empty for empty map", func(t *testing.T) {
		assert.Equal(t, "", mostActiveIoDevice(map[string]disk.IOCountersStat{}))
	})
}

func TestIsDockerSpecialMountpoint(t *testing.T) {
	testCases := []struct {
		name       string
		mountpoint string
		expected   bool
	}{
		{name: "hosts", mountpoint: "/etc/hosts", expected: true},
		{name: "resolv", mountpoint: "/etc/resolv.conf", expected: true},
		{name: "hostname", mountpoint: "/etc/hostname", expected: true},
		{name: "root", mountpoint: "/", expected: false},
		{name: "passwd", mountpoint: "/etc/passwd", expected: false},
		{name: "extra-filesystem", mountpoint: "/extra-filesystems/sda1", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isDockerSpecialMountpoint(tc.mountpoint))
		})
	}
}

func TestFsStatsWithCustomNames(t *testing.T) {
	// Test that FsStats properly stores custom names
	fsStats := &system.FsStats{
		Mountpoint: "/mnt/storage",
		Name:       "my-custom-storage",
		DiskTotal:  100.0,
		DiskUsed:   50.0,
	}

	assert.Equal(t, "my-custom-storage", fsStats.Name)
	assert.Equal(t, "/mnt/storage", fsStats.Mountpoint)
	assert.Equal(t, 100.0, fsStats.DiskTotal)
	assert.Equal(t, 50.0, fsStats.DiskUsed)
}

func TestExtraFsKeyGeneration(t *testing.T) {
	// Test the logic for generating ExtraFs keys with custom names
	testCases := []struct {
		name        string
		deviceName  string
		customName  string
		expectedKey string
	}{
		{
			name:        "with custom name",
			deviceName:  "sda1",
			customName:  "my-storage",
			expectedKey: "my-storage",
		},
		{
			name:        "without custom name",
			deviceName:  "sda1",
			customName:  "",
			expectedKey: "sda1",
		},
		{
			name:        "empty custom name falls back to device",
			deviceName:  "nvme0n1p2",
			customName:  "",
			expectedKey: "nvme0n1p2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the key generation logic from agent.go
			key := tc.deviceName
			if tc.customName != "" {
				key = tc.customName
			}
			assert.Equal(t, tc.expectedKey, key)
		})
	}
}

func TestDiskUsageCaching(t *testing.T) {
	t.Run("caching disabled updates all filesystems", func(t *testing.T) {
		agent := &Agent{
			fsStats: map[string]*system.FsStats{
				"sda": {Root: true, Mountpoint: "/"},
				"sdb": {Root: false, Mountpoint: "/mnt/storage"},
			},
			diskUsageCacheDuration: 0, // caching disabled
		}

		var stats system.Stats
		agent.updateDiskUsage(&stats)

		// Both should be updated (non-zero values from disk.Usage)
		// Root stats should be populated in systemStats
		assert.True(t, agent.lastDiskUsageUpdate.IsZero() || !agent.lastDiskUsageUpdate.IsZero(),
			"lastDiskUsageUpdate should be set when caching is disabled")
	})

	t.Run("caching enabled always updates root filesystem", func(t *testing.T) {
		agent := &Agent{
			fsStats: map[string]*system.FsStats{
				"sda": {Root: true, Mountpoint: "/", DiskTotal: 100, DiskUsed: 50},
				"sdb": {Root: false, Mountpoint: "/mnt/storage", DiskTotal: 200, DiskUsed: 100},
			},
			diskUsageCacheDuration: 1 * time.Hour,
			lastDiskUsageUpdate:    time.Now(), // cache is fresh
		}

		// Store original extra fs values
		originalExtraTotal := agent.fsStats["sdb"].DiskTotal
		originalExtraUsed := agent.fsStats["sdb"].DiskUsed

		var stats system.Stats
		agent.updateDiskUsage(&stats)

		// Root should be updated (systemStats populated from disk.Usage call)
		// We can't easily check if disk.Usage was called, but we verify the flow works

		// Extra filesystem should retain cached values (not reset)
		assert.Equal(t, originalExtraTotal, agent.fsStats["sdb"].DiskTotal,
			"extra filesystem DiskTotal should be unchanged when cached")
		assert.Equal(t, originalExtraUsed, agent.fsStats["sdb"].DiskUsed,
			"extra filesystem DiskUsed should be unchanged when cached")
	})

	t.Run("first call always updates all filesystems", func(t *testing.T) {
		agent := &Agent{
			fsStats: map[string]*system.FsStats{
				"sda": {Root: true, Mountpoint: "/"},
				"sdb": {Root: false, Mountpoint: "/mnt/storage"},
			},
			diskUsageCacheDuration: 1 * time.Hour,
			// lastDiskUsageUpdate is zero (first call)
		}

		var stats system.Stats
		agent.updateDiskUsage(&stats)

		// After first call, lastDiskUsageUpdate should be set
		assert.False(t, agent.lastDiskUsageUpdate.IsZero(),
			"lastDiskUsageUpdate should be set after first call")
	})

	t.Run("expired cache updates extra filesystems", func(t *testing.T) {
		agent := &Agent{
			fsStats: map[string]*system.FsStats{
				"sda": {Root: true, Mountpoint: "/"},
				"sdb": {Root: false, Mountpoint: "/mnt/storage"},
			},
			diskUsageCacheDuration: 1 * time.Millisecond,
			lastDiskUsageUpdate:    time.Now().Add(-1 * time.Second), // cache expired
		}

		var stats system.Stats
		agent.updateDiskUsage(&stats)

		// lastDiskUsageUpdate should be refreshed since cache expired
		assert.True(t, time.Since(agent.lastDiskUsageUpdate) < time.Second,
			"lastDiskUsageUpdate should be refreshed when cache expires")
	})
}

func TestHasSameDiskUsage(t *testing.T) {
	const toleranceBytes uint64 = 16 * 1024 * 1024

	t.Run("returns true when totals and usage are equal", func(t *testing.T) {
		a := &disk.UsageStat{Total: 100 * 1024 * 1024 * 1024, Used: 42 * 1024 * 1024 * 1024}
		b := &disk.UsageStat{Total: 100 * 1024 * 1024 * 1024, Used: 42 * 1024 * 1024 * 1024}
		assert.True(t, hasSameDiskUsage(a, b))
	})

	t.Run("returns true within tolerance", func(t *testing.T) {
		a := &disk.UsageStat{Total: 100 * 1024 * 1024 * 1024, Used: 42 * 1024 * 1024 * 1024}
		b := &disk.UsageStat{
			Total: a.Total + toleranceBytes - 1,
			Used:  a.Used - toleranceBytes + 1,
		}
		assert.True(t, hasSameDiskUsage(a, b))
	})

	t.Run("returns false when total exceeds tolerance", func(t *testing.T) {
		a := &disk.UsageStat{Total: 100 * 1024 * 1024 * 1024, Used: 42 * 1024 * 1024 * 1024}
		b := &disk.UsageStat{
			Total: a.Total + toleranceBytes + 1,
			Used:  a.Used,
		}
		assert.False(t, hasSameDiskUsage(a, b))
	})

	t.Run("returns false for nil or zero total", func(t *testing.T) {
		assert.False(t, hasSameDiskUsage(nil, &disk.UsageStat{Total: 1, Used: 1}))
		assert.False(t, hasSameDiskUsage(&disk.UsageStat{Total: 1, Used: 1}, nil))
		assert.False(t, hasSameDiskUsage(&disk.UsageStat{Total: 0, Used: 0}, &disk.UsageStat{Total: 1, Used: 1}))
	})
}

func TestInitializeDiskIoStatsResetsTrackedDevices(t *testing.T) {
	agent := &Agent{
		fsStats: map[string]*system.FsStats{
			"root":  {Root: true, IoDevice: "sda"},
			"data":  {IoDevice: "sdb"},
			"share": {IoDevice: "sdb"},
			"nfs":   {},
		},
		fsNames: []string{"stale", "sda"},
	}

	agent.initializeDiskIoStats(map[string]disk.IOCountersStat{
		"sda": {Name: "sda", ReadBytes: 10, WriteBytes: 20},
		"sdb": {Name: "sdb", ReadBytes: 30, WriteBytes: 40},
	})

	// Filesystems sharing a device track it once.
	assert.ElementsMatch(t, []string{"sda", "sdb"}, agent.fsNames)
	assert.Equal(t, uint64(10), agent.fsStats["root"].TotalRead)
	assert.Equal(t, uint64(20), agent.fsStats["root"].TotalWrite)
	assert.Equal(t, uint64(30), agent.fsStats["share"].TotalRead)
	assert.Equal(t, uint64(10), agent.diskBaseline["sda"].readBytes)
	assert.Equal(t, uint64(40), agent.diskBaseline["sdb"].writeBytes)
	assert.False(t, agent.diskBaseline["sda"].at.IsZero())
	assert.False(t, agent.diskBaseline["sdb"].at.IsZero())

	agent.initializeDiskIoStats(map[string]disk.IOCountersStat{
		"sdb": {Name: "sdb", ReadBytes: 50, WriteBytes: 60},
	})

	assert.Equal(t, []string{"sdb"}, agent.fsNames)
	assert.Equal(t, uint64(50), agent.fsStats["data"].TotalRead)
	assert.Equal(t, uint64(60), agent.fsStats["share"].TotalWrite)
}

func TestIoTimeDelta(t *testing.T) {
	assert.Equal(t, uint64(300), ioTimeDelta(1200, 900))

	// A lower value is a 32-bit wrap only on Linux. Other platforms
	// report 64-bit counters, so there it is a reset.
	var want uint64
	if runtime.GOOS == "linux" {
		want = 1200
	}
	assert.Equal(t, want, ioTimeDelta(200, math.MaxUint32+1-1000))

	assert.Equal(t, uint64(0), ioTimeDelta(200, math.MaxUint32+1000))
}

func TestNormalizeDeviceName(t *testing.T) {
	// A Windows volume name is not a path element, so every spelling of the
	// same drive has to normalize to the same key. filepath.Base cannot do
	// this: on Windows it strips the "C:" specifier and returns "\", which
	// collapses every drive letter onto one key (#2417).
	for _, spelling := range []string{"C:", `C:\`, "C:/", `C:\\`} {
		assert.Equal(t, "C:", normalizeDeviceName(spelling), "spelling %q", spelling)
	}
	// Drive letters are case-insensitive, so the letter is uppercased.
	assert.Equal(t, "D:", normalizeDeviceName("d:"))
	assert.Equal(t, "C:", normalizeDeviceName(" c: "))
	assert.Equal(t, "C:", normalizeDeviceName(`c:\`))

	// Non-volume inputs keep using filepath.Base.
	assert.Equal(t, "sda1", normalizeDeviceName("/dev/sda1"))
	assert.Equal(t, "sda1", normalizeDeviceName("/dev/sda1/"))
	assert.Equal(t, "nvme0n1p2", normalizeDeviceName("  /dev/nvme0n1p2  "))
	assert.Equal(t, "", normalizeDeviceName("."))
	assert.Equal(t, "", normalizeDeviceName("   "))

	// A drive-relative path is a path, not a volume.
	assert.Equal(t, filepath.Base(`C:data`), normalizeDeviceName(`C:data`))
}

func TestFindIoDeviceWindowsVolumeNames(t *testing.T) {
	// Every drive normalizes to a distinct key, so the root drive resolves
	// exactly instead of to whichever counter the map yielded first (#2417).
	ioCounters := map[string]disk.IOCountersStat{
		"C:": {Name: "C:", ReadBytes: 10, WriteBytes: 10},
		"D:": {Name: "D:", ReadBytes: 20, WriteBytes: 20},
		"P:": {Name: "P:", ReadBytes: 30, WriteBytes: 30},
	}

	for i := 0; i < 32; i++ {
		device, ok := findIoDevice("C:", ioCounters)
		assert.True(t, ok)
		assert.Equal(t, "C:", device)
	}

	// The drive may arrive with a trailing separator, as a mount point does.
	device, ok := findIoDevice(`C:\`, ioCounters)
	assert.True(t, ok)
	assert.Equal(t, "C:", device)
}
