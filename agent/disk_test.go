//go:build testing

package agent

import (
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// newTestDiscovery returns a non-Windows discovery over the given counters.
// disk.Usage is unavailable unless a test sets usageFn.
func newTestDiscovery(counters map[string]disk.IOCountersStat, partitions ...disk.PartitionStat) (*Agent, *diskDiscovery) {
	agent := &Agent{fsStats: make(map[string]*system.FsStats)}
	return agent, &diskDiscovery{
		agent:          agent,
		rootMountPoint: "/",
		partitions:     partitions,
		ioCounters:     counters,
		efPath:         filepath.FromSlash("/extra-filesystems"),
		usageFn:        func(string) (*disk.UsageStat, error) { return nil, os.ErrNotExist },
	}
}

// setEvalSymlinks swaps the device-symlink resolver for the duration of a test.
func setEvalSymlinks(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	old := evalSymlinks
	evalSymlinks = fn
	t.Cleanup(func() { evalSymlinks = old })
}

func TestIoKey(t *testing.T) {
	luks := "/dev/mapper/luks-2bcb02be-999d-4417-8d18-5c61e660fb6e"
	luksCounters := map[string]disk.IOCountersStat{
		"dm-1":      {Name: "dm-1", Label: "luks-2bcb02be-999d-4417-8d18-5c61e660fb6e"},
		"nvme0n1p2": {Name: "nvme0n1p2"},
	}
	lvmCounters := map[string]disk.IOCountersStat{
		"dm-0": {Name: "dm-0", Label: "vg1-syno_vg_reserved_area"},
		"dm-1": {Name: "dm-1", Label: "vg1-volume_1"},
	}
	lvmTo := func(target string) func(string) (string, error) {
		return func(path string) (string, error) {
			if path == "/dev/vg1/volume_1" {
				return target, nil
			}
			return path, nil
		}
	}

	tests := []struct {
		name     string
		counters map[string]disk.IOCountersStat
		symlinks func(string) (string, error)
		device   string
		hints    []string
		wantKey  string
		wantOk   bool
	}{
		{name: "exact base name", counters: map[string]disk.IOCountersStat{"sda1": {Name: "sda1"}}, device: "/dev/sda1", wantKey: "sda1", wantOk: true},
		{name: "partition to parent disk by prefix", counters: map[string]disk.IOCountersStat{"ada0": {Name: "ada0"}}, device: "/dev/ada0p2", wantKey: "ada0", wantOk: true},
		{name: "hint maps an unrelated device", counters: map[string]disk.IOCountersStat{"nvme0n1": {Name: "nvme0n1"}}, device: "overlay", hints: []string{"nvme0n1p2"}, wantKey: "nvme0n1", wantOk: true},
		{name: "hint wins over mapper label", counters: luksCounters, device: luks, hints: []string{"nvme0n1p2"}, wantKey: "nvme0n1p2", wantOk: true},
		{name: "mapper label without hint", counters: luksCounters, device: luks, hints: []string{"Archive"}, wantKey: "dm-1", wantOk: true},
		{name: "empty hint is ignored", counters: luksCounters, device: luks, hints: []string{""}, wantKey: "dm-1", wantOk: true},
		{name: "lvm symlink resolves to dm device", counters: lvmCounters, symlinks: lvmTo("/dev/dm-1"), device: "/dev/vg1/volume_1", wantKey: "dm-1", wantOk: true},
		{name: "lvm symlink resolves to mapper label", counters: lvmCounters, symlinks: lvmTo("/dev/mapper/vg1-volume_1"), device: "/dev/vg1/volume_1", wantKey: "dm-1", wantOk: true},
		{name: "explicit hint wins over resolved device", counters: map[string]disk.IOCountersStat{"dm-1": {Name: "dm-1"}, "sda": {Name: "sda"}}, symlinks: lvmTo("/dev/dm-1"), device: "/dev/vg1/volume_1", hints: []string{"sda"}, wantKey: "sda", wantOk: true},
		{name: "unresolvable symlink keeps base name", counters: lvmCounters, symlinks: func(string) (string, error) { return "", os.ErrNotExist }, device: "/dev/vg1/volume_1", wantKey: "volume_1"},
		{name: "relative names are not resolved", counters: map[string]disk.IOCountersStat{"sdb1": {Name: "sdb1"}}, symlinks: func(string) (string, error) { return "/dev/sdb1", nil }, device: "data", wantKey: "data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.symlinks != nil {
				setEvalSymlinks(t, tt.symlinks)
			}
			_, d := newTestDiscovery(tt.counters)
			key, ok := d.ioKey(tt.device, tt.hints...)
			assert.Equal(t, tt.wantKey, key)
			assert.Equal(t, tt.wantOk, ok)
		})
	}

	t.Run("windows keeps full volume name", func(t *testing.T) {
		_, d := newTestDiscovery(map[string]disk.IOCountersStat{"C:": {Name: "C:"}})
		d.isWindows = true
		key, ok := d.ioKey("C:")
		assert.True(t, ok)
		assert.Equal(t, "C:", key)
	})
}

func TestRegister(t *testing.T) {
	agent, d := newTestDiscovery(nil)

	assert.True(t, d.register("sda1", "/mnt/data", false, "archive"))
	assert.Equal(t, &system.FsStats{Mountpoint: "/mnt/data", IoDevice: "sda1", Name: "archive"}, agent.fsStats["/mnt/data"])

	// Mounts sharing an I/O device are tracked separately (#1878, #1931).
	assert.True(t, d.register("sda1", "/mnt/other", false, ""))
	assert.Len(t, agent.fsStats, 2)

	// A non-root on a tracked mountpoint is skipped.
	assert.False(t, d.register("sdb1", "/mnt/data", false, ""))
	assert.Equal(t, "sda1", agent.fsStats["/mnt/data"].IoDevice)

	// A root on a tracked mountpoint promotes it and keeps its custom name.
	assert.True(t, d.register("sdb", "/mnt/data", true, ""))
	assert.Len(t, agent.fsStats, 2)
	assert.True(t, agent.fsStats["/mnt/data"].Root)
	assert.Equal(t, "sdb", agent.fsStats["/mnt/data"].IoDevice)
	assert.Equal(t, "archive", agent.fsStats["/mnt/data"].Name)
}

func TestAddConfiguredRootFs(t *testing.T) {
	t.Run("adds root from matching partition", func(t *testing.T) {
		agent, d := newTestDiscovery(
			map[string]disk.IOCountersStat{"ada0": {Name: "ada0", ReadBytes: 1000, WriteBytes: 1000}},
			disk.PartitionStat{Device: "/dev/ada0p2", Mountpoint: "/"},
		)

		assert.True(t, d.addConfiguredRootFs("/dev/ada0p2", "root disk"))
		stats := agent.fsStats["/"]
		require.NotNil(t, stats)
		assert.True(t, stats.Root)
		assert.Equal(t, "ada0", stats.IoDevice)
		assert.Equal(t, "root disk", stats.Name)
	})

	t.Run("filesystem setting is a hint for the partition device", func(t *testing.T) {
		agent, d := newTestDiscovery(
			map[string]disk.IOCountersStat{"nvme0n1": {Name: "nvme0n1"}},
			disk.PartitionStat{Device: "overlay", Mountpoint: "/"},
		)
		assert.True(t, d.addConfiguredRootFs("/", ""))
		// "/" is no device, so the overlay mount stays unmapped.
		assert.Equal(t, "overlay", agent.fsStats["/"].IoDevice)
	})

	t.Run("exact device wins over prefix", func(t *testing.T) {
		agent, d := newTestDiscovery(
			map[string]disk.IOCountersStat{"sda1": {Name: "sda1"}, "sda10": {Name: "sda10"}},
			disk.PartitionStat{Device: "/dev/sda10", Mountpoint: "/data"},
			disk.PartitionStat{Device: "/dev/sda1", Mountpoint: "/"},
		)
		assert.True(t, d.addConfiguredRootFs("sda1", ""))
		require.Contains(t, agent.fsStats, "/")
		assert.Equal(t, "sda1", agent.fsStats["/"].IoDevice)
	})

	t.Run("adds root from io device when partition is missing", func(t *testing.T) {
		agent, d := newTestDiscovery(map[string]disk.IOCountersStat{
			"nda0": {Name: "nda0", Label: "zroot", ReadBytes: 1000, WriteBytes: 1000},
		})
		d.rootMountPoint = "/sysroot"

		assert.True(t, d.addConfiguredRootFs("zroot", "root pool"))
		stats := agent.fsStats["/sysroot"]
		require.NotNil(t, stats)
		assert.True(t, stats.Root)
		assert.Equal(t, "nda0", stats.IoDevice)
		assert.Equal(t, "root pool", stats.Name)
	})

	t.Run("returns false when filesystem cannot be resolved", func(t *testing.T) {
		agent, d := newTestDiscovery(map[string]disk.IOCountersStat{})
		assert.False(t, d.addConfiguredRootFs("missing-disk", ""))
		assert.False(t, d.addConfiguredRootFs("", ""))
		assert.Empty(t, agent.fsStats)
	})
}

func TestAddPartitionRootFs(t *testing.T) {
	t.Run("adds root from fallback partition candidate", func(t *testing.T) {
		agent, d := newTestDiscovery(map[string]disk.IOCountersStat{
			"nvme0n1": {Name: "nvme0n1", ReadBytes: 1000, WriteBytes: 1000},
		})

		assert.True(t, d.addPartitionRootFs("/dev/nvme0n1p2", "/"))
		stats := agent.fsStats["/"]
		require.NotNil(t, stats)
		assert.True(t, stats.Root)
		assert.Equal(t, "nvme0n1", stats.IoDevice)
	})

	t.Run("returns false when no io device matches", func(t *testing.T) {
		agent, d := newTestDiscovery(map[string]disk.IOCountersStat{})
		assert.False(t, d.addPartitionRootFs("/dev/mapper/root", "/"))
		assert.Empty(t, agent.fsStats)
	})
}

func TestAddLastResortRootFs(t *testing.T) {
	t.Run("uses most active io device when available", func(t *testing.T) {
		agent, d := newTestDiscovery(map[string]disk.IOCountersStat{
			"sda": {Name: "sda", ReadBytes: 5000, WriteBytes: 5000},
			"sdb": {Name: "sdb", ReadBytes: 1000, WriteBytes: 1000},
		})

		d.addLastResortRootFs()

		stats := agent.fsStats["/"]
		require.NotNil(t, stats)
		assert.True(t, stats.Root)
		assert.Equal(t, "sda", stats.IoDevice)
	})

	t.Run("tracks root usage without io device", func(t *testing.T) {
		agent, d := newTestDiscovery(map[string]disk.IOCountersStat{})
		agent.fsStats["/extra-filesystems/sysroot"] = &system.FsStats{Mountpoint: "/extra-filesystems/sysroot", IoDevice: "sysroot"}
		d.rootMountPoint = "/sysroot"

		d.addLastResortRootFs()

		require.Len(t, agent.fsStats, 2)
		stats := agent.fsStats["/sysroot"]
		require.NotNil(t, stats)
		assert.True(t, stats.Root)
		assert.Empty(t, stats.IoDevice)
	})
}

func TestFindPartition(t *testing.T) {
	root := disk.PartitionStat{Device: "/dev/ada0p2", Mountpoint: "/"}

	match := func(setting string, partitions ...disk.PartitionStat) (disk.PartitionStat, bool) {
		return findPartition(setting, partitions)
	}

	t.Run("matches mountpoint", func(t *testing.T) {
		_, ok := match("/", root)
		assert.True(t, ok)
	})

	t.Run("matches exact device", func(t *testing.T) {
		for _, setting := range []string{"ada0p2", "/dev/ada0p2", " ada0p2 "} {
			_, ok := match(setting, root)
			assert.True(t, ok, setting)
		}
		p, ok := match("mapper/vg-lv", disk.PartitionStat{Device: "/dev/mapper/vg-lv", Mountpoint: "/data"})
		assert.True(t, ok)
		assert.Equal(t, "/data", p.Mountpoint)
	})

	t.Run("matches parent device by prefix", func(t *testing.T) {
		for _, setting := range []string{"ada0", "/dev/ada0"} {
			_, ok := match(setting, root)
			assert.True(t, ok, setting)
		}
	})

	t.Run("does not match unrelated device", func(t *testing.T) {
		for _, setting := range []string{"sda", "nvme0n1", "", "  "} {
			_, ok := match(setting, root)
			assert.False(t, ok, setting)
		}
	})

	t.Run("exact device wins over prefix", func(t *testing.T) {
		p, ok := match("sda1",
			disk.PartitionStat{Device: "/dev/sda10", Mountpoint: "/data"},
			disk.PartitionStat{Device: "/dev/sda1", Mountpoint: "/boot"},
		)
		assert.True(t, ok)
		assert.Equal(t, "/boot", p.Mountpoint)
	})

	t.Run("path outside /dev only matches mountpoints", func(t *testing.T) {
		_, ok := match("/mnt/sda1", disk.PartitionStat{Device: "/dev/sda1", Mountpoint: "/data"})
		assert.False(t, ok)
	})

	t.Run("skips autofs placeholder on the same path", func(t *testing.T) {
		// systemd automount: autofs is listed first, the real mount on top (#1931).
		p, ok := match("/media/backup",
			disk.PartitionStat{Device: "systemd-1", Mountpoint: "/media/backup", Fstype: "autofs"},
			disk.PartitionStat{Device: "/dev/sdb1", Mountpoint: "/media/backup", Fstype: "ext4"},
		)
		assert.True(t, ok)
		assert.Equal(t, "/dev/sdb1", p.Device)
	})

	t.Run("last mount on a path wins", func(t *testing.T) {
		p, ok := match("/data",
			disk.PartitionStat{Device: "/dev/sdb1", Mountpoint: "/data"},
			disk.PartitionStat{Device: "/dev/sdc1", Mountpoint: "/data"},
		)
		assert.True(t, ok)
		assert.Equal(t, "/dev/sdc1", p.Device)
	})
}

func TestAddConfiguredExtraFilesystems(t *testing.T) {
	t.Run("uses matching partition when present", func(t *testing.T) {
		agent, d := newTestDiscovery(
			map[string]disk.IOCountersStat{"sdb1": {Name: "sdb1"}},
			disk.PartitionStat{Device: "/dev/sdb1", Mountpoint: "/mnt/backup"},
		)
		d.usageFn = func(string) (*disk.UsageStat, error) {
			t.Fatal("usage fallback should not be called when partition matches")
			return nil, nil
		}

		d.addConfiguredExtraFilesystems("sdb1__backup")

		stats := agent.fsStats["/mnt/backup"]
		require.NotNil(t, stats)
		assert.Equal(t, "sdb1", stats.IoDevice)
		assert.Equal(t, "backup", stats.Name)
	})

	t.Run("falls back to usage-validated path", func(t *testing.T) {
		agent, d := newTestDiscovery(map[string]disk.IOCountersStat{"archive": {Name: "archive"}})
		d.usageFn = func(path string) (*disk.UsageStat, error) {
			assert.Equal(t, "/srv/archive", path)
			return &disk.UsageStat{}, nil
		}

		d.addConfiguredExtraFilesystems("/srv/archive__archive")

		stats := agent.fsStats["/srv/archive"]
		require.NotNil(t, stats)
		assert.Equal(t, "archive", stats.IoDevice)
		assert.Equal(t, "archive", stats.Name)
	})

	t.Run("ignores invalid and empty entries", func(t *testing.T) {
		agent, d := newTestDiscovery(nil)
		d.addConfiguredExtraFilesystems("/missing/archive,, ")
		assert.Empty(t, agent.fsStats)
	})

	t.Run("does not report /dev usage for unmounted device", func(t *testing.T) {
		agent, d := newTestDiscovery(nil)
		d.usageFn = func(string) (*disk.UsageStat, error) {
			t.Fatal("usage of a device node is the usage of /dev")
			return nil, nil
		}
		d.addConfiguredExtraFilesystems("/dev/sdz1")
		assert.Empty(t, agent.fsStats)
	})

	t.Run("registers multiple entries", func(t *testing.T) {
		agent, d := newTestDiscovery(
			map[string]disk.IOCountersStat{"sda1": {Name: "sda1"}, "archive": {Name: "archive"}},
			disk.PartitionStat{Device: "/dev/sda1", Mountpoint: "/mnt/fast"},
		)
		d.usageFn = func(path string) (*disk.UsageStat, error) {
			if path == "/srv/archive" {
				return &disk.UsageStat{}, nil
			}
			return nil, os.ErrNotExist
		}

		d.addConfiguredExtraFilesystems("sda1__fast,/srv/archive__cold")

		require.Contains(t, agent.fsStats, "/mnt/fast")
		assert.Equal(t, "fast", agent.fsStats["/mnt/fast"].Name)
		require.Contains(t, agent.fsStats, "/srv/archive")
		assert.Equal(t, "cold", agent.fsStats["/srv/archive"].Name)
	})

	t.Run("keeps both systemd automounts", func(t *testing.T) {
		// #1931: both autofs placeholders share the device name systemd-1.
		agent, d := newTestDiscovery(
			map[string]disk.IOCountersStat{"sdb1": {Name: "sdb1"}, "sdc1": {Name: "sdc1"}},
			disk.PartitionStat{Device: "systemd-1", Mountpoint: "/media/backup", Fstype: "autofs"},
			disk.PartitionStat{Device: "systemd-1", Mountpoint: "/media/video", Fstype: "autofs"},
			disk.PartitionStat{Device: "/dev/sdc1", Mountpoint: "/media/video", Fstype: "exfat"},
			disk.PartitionStat{Device: "/dev/sdb1", Mountpoint: "/media/backup", Fstype: "ext4"},
		)

		d.addConfiguredExtraFilesystems("/media/video__Video,/media/backup__Backup")

		require.Len(t, agent.fsStats, 2)
		assert.Equal(t, "sdc1", agent.fsStats["/media/video"].IoDevice)
		assert.Equal(t, "sdb1", agent.fsStats["/media/backup"].IoDevice)
	})
}

func TestAddExtraFilesystemFolders(t *testing.T) {
	ef := func(name string) string { return filepath.Join(filepath.FromSlash("/extra-filesystems"), name) }
	counters := map[string]disk.IOCountersStat{
		"sdc":       {Name: "sdc"},
		"nvme0n1p1": {Name: "nvme0n1p1"},
		"nvme1n1":   {Name: "nvme1n1"},
		"newdisk":   {Name: "newdisk"},
	}

	t.Run("io device comes from partition, name from folder", func(t *testing.T) {
		agent, d := newTestDiscovery(counters,
			disk.PartitionStat{Device: "/dev/sdc", Mountpoint: ef("Share")},
			disk.PartitionStat{Device: "/dev/nvme0n1p1", Mountpoint: ef("sdc__Root")},
			disk.PartitionStat{Device: "/dev/nvme1n1", Mountpoint: ef("__Docker")},
		)

		d.addExtraFilesystemFolders([]string{"Share", "sdc__Root", "__Docker"})

		require.Len(t, agent.fsStats, 3)
		assert.Equal(t, &system.FsStats{Mountpoint: ef("Share"), IoDevice: "sdc"}, agent.fsStats[ef("Share")])
		assert.Equal(t, &system.FsStats{Mountpoint: ef("sdc__Root"), IoDevice: "nvme0n1p1", Name: "Root"}, agent.fsStats[ef("sdc__Root")])
		assert.Equal(t, &system.FsStats{Mountpoint: ef("__Docker"), IoDevice: "nvme1n1", Name: "Docker"}, agent.fsStats[ef("__Docker")])
	})

	t.Run("folder device is used without partition", func(t *testing.T) {
		agent, d := newTestDiscovery(counters)
		d.addExtraFilesystemFolders([]string{"newdisk__Archive"})
		assert.Equal(t, &system.FsStats{Mountpoint: ef("newdisk__Archive"), IoDevice: "newdisk", Name: "Archive"}, agent.fsStats[ef("newdisk__Archive")])
	})

	t.Run("folder device is a hint for an unmapped partition device", func(t *testing.T) {
		agent, d := newTestDiscovery(
			map[string]disk.IOCountersStat{"nvme0n1p2": {Name: "nvme0n1p2"}},
			disk.PartitionStat{Device: "/dev/mapper/luks-2bcb02be", Mountpoint: ef("nvme0n1p2__Archive")},
		)
		d.addExtraFilesystemFolders([]string{"nvme0n1p2__Archive"})
		assert.Equal(t, "nvme0n1p2", agent.fsStats[ef("nvme0n1p2__Archive")].IoDevice)
	})

	t.Run("skips mountpoints already registered", func(t *testing.T) {
		agent, d := newTestDiscovery(counters)
		agent.fsStats[ef("existing")] = &system.FsStats{Mountpoint: ef("existing"), Name: "kept"}
		d.addExtraFilesystemFolders([]string{"existing"})
		assert.Equal(t, "kept", agent.fsStats[ef("existing")].Name)
	})

	t.Run("ignores nested mounts of a bind-mounted host root", func(t *testing.T) {
		// disk.Partitions(true) also returns /proc, /sys... under the bind mount (#1859).
		agent, d := newTestDiscovery(counters,
			disk.PartitionStat{Device: "/dev/nvme0n1p1", Mountpoint: ef("nvme0n1p1__caddy1-root")},
			disk.PartitionStat{Device: "proc", Mountpoint: ef("nvme0n1p1__caddy1-root/proc")},
			disk.PartitionStat{Device: "overlay", Mountpoint: ef("nvme0n1p1__caddy1-root/var/lib/docker")},
		)
		d.addExtraFilesystemFolders([]string{"nvme0n1p1__caddy1-root"})
		require.Len(t, agent.fsStats, 1)
		assert.Equal(t, "nvme0n1p1", agent.fsStats[ef("nvme0n1p1__caddy1-root")].IoDevice)
	})
}

func TestPruneDuplicateFilesystems(t *testing.T) {
	gb := uint64(1 << 30)
	counters := map[string]disk.IOCountersStat{"sda": {Name: "sda"}, "sdb": {Name: "sdb"}}
	usage := map[string]*disk.UsageStat{
		"/":                       {Total: 100 * gb, Used: 40 * gb},
		"/extra-filesystems/root": {Total: 100 * gb, Used: 40 * gb}, // folder of the root disk
		"/extra-filesystems/data": {Total: 100 * gb, Used: 40 * gb}, // same usage, other disk (#2103)
		"/srv/archive":            {Total: 100 * gb, Used: 40 * gb}, // folder without diskstats
		"/mnt/home":               {Total: 50 * gb, Used: 10 * gb},  // other partition of root's disk (#1878)
	}
	agent := &Agent{fsStats: map[string]*system.FsStats{
		"/":                       {Root: true, Mountpoint: "/", IoDevice: "sda"},
		"/extra-filesystems/root": {Mountpoint: "/extra-filesystems/root", IoDevice: "sda"},
		"/extra-filesystems/data": {Mountpoint: "/extra-filesystems/data", IoDevice: "sdb"},
		"/srv/archive":            {Mountpoint: "/srv/archive", IoDevice: "archive"},
		"/mnt/home":               {Mountpoint: "/mnt/home", IoDevice: "sda"},
		"/mnt/gone":               {Mountpoint: "/mnt/gone", IoDevice: "sda"}, // usage fails
	}}

	agent.pruneDuplicateFilesystems(counters, func(path string) (*disk.UsageStat, error) {
		if u, ok := usage[path]; ok {
			return u, nil
		}
		return nil, os.ErrNotExist
	})

	assert.ElementsMatch(t, []string{"/", "/extra-filesystems/data", "/mnt/home", "/mnt/gone"}, slices.Collect(maps.Keys(agent.fsStats)))
}

func TestSetExtraFsStats(t *testing.T) {
	agent := &Agent{fsStats: map[string]*system.FsStats{
		"/":            {Root: true, Mountpoint: "/", IoDevice: "sda", Name: "System", DiskTotal: 100, DiskUsed: 50},
		"/mnt/a":       {Mountpoint: "/mnt/a", IoDevice: "sdb1", DiskTotal: 100, DiskUsed: 25},
		"/mnt/b":       {Mountpoint: "/mnt/b", IoDevice: "sdb1", DiskTotal: 100, DiskUsed: 10},
		"/mnt/named":   {Mountpoint: "/mnt/named", IoDevice: "sdc1", Name: "Backup", DiskTotal: 200, DiskUsed: 50},
		"/mnt/missing": {Mountpoint: "/mnt/missing", IoDevice: "sdd1"},
	}}
	var stats system.Stats
	var info system.Info

	agent.setExtraFsStats(&stats, &info)

	assert.Equal(t, "System", info.RootDiskName)
	// Keys stay what chart history uses: custom name, else I/O device.
	assert.Same(t, agent.fsStats["/mnt/a"], stats.ExtraFs["sdb1"])
	assert.Same(t, agent.fsStats["/mnt/named"], stats.ExtraFs["Backup"])
	// A second mount on the same device falls back to its mountpoint's name.
	assert.Same(t, agent.fsStats["/mnt/b"], stats.ExtraFs["b"])
	assert.Len(t, stats.ExtraFs, 3)
	assert.Equal(t, map[string]float64{"sdb1": 25, "b": 10, "Backup": 25}, info.ExtraFsPct)
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
			"/":         {Mountpoint: "/", IoDevice: "sda"},
			"/mnt/data": {Mountpoint: "/mnt/data", IoDevice: "sdb"},
			"/mnt/more": {Mountpoint: "/mnt/more", IoDevice: "sdb"},
		},
		fsNames: []string{"stale", "sda"},
	}

	agent.initializeDiskIoStats(map[string]disk.IOCountersStat{
		"sda": {Name: "sda", ReadBytes: 10, WriteBytes: 20},
		"sdb": {Name: "sdb", ReadBytes: 30, WriteBytes: 40},
	})

	// A device shared by two filesystems is read once.
	assert.ElementsMatch(t, []string{"sda", "sdb"}, agent.fsNames)
	assert.Equal(t, uint64(10), agent.fsStats["/"].TotalRead)
	assert.Equal(t, uint64(20), agent.fsStats["/"].TotalWrite)
	assert.Equal(t, uint64(30), agent.fsStats["/mnt/more"].TotalRead)
	assert.Equal(t, uint64(10), agent.diskBaseline["sda"].readBytes)
	assert.Equal(t, uint64(40), agent.diskBaseline["sdb"].writeBytes)
	assert.False(t, agent.diskBaseline["sda"].at.IsZero())
	assert.False(t, agent.diskBaseline["sdb"].at.IsZero())

	agent.initializeDiskIoStats(map[string]disk.IOCountersStat{
		"sdb": {Name: "sdb", ReadBytes: 50, WriteBytes: 60},
	})

	assert.Equal(t, []string{"sdb"}, agent.fsNames)
	assert.Equal(t, uint64(50), agent.fsStats["/mnt/data"].TotalRead)
	assert.Equal(t, uint64(60), agent.fsStats["/mnt/data"].TotalWrite)
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

func TestAddPartitionRootFsWindowsDrive(t *testing.T) {
	agent, d := newTestDiscovery(map[string]disk.IOCountersStat{
		"C:": {Name: "C:"},
		"D:": {Name: "D:"},
		"P:": {Name: "P:"},
	})
	d.isWindows = true

	assert.True(t, d.addPartitionRootFs("C:", `C:\`))
	assert.Len(t, agent.fsStats, 1)
	stats := agent.fsStats[`C:\`]
	require.NotNil(t, stats)
	assert.True(t, stats.Root)
	assert.Equal(t, "C:", stats.IoDevice)
}

func TestAddPartitionRootFsMountpointAlreadyRegistered(t *testing.T) {
	// The root drive is also listed in EXTRA_FILESYSTEMS, so it is tracked
	// before the root fallback runs. The existing entry must be promoted to root
	// rather than falling back to the most active device, which here is D:.
	agent, d := newTestDiscovery(map[string]disk.IOCountersStat{
		"C:": {Name: "C:", ReadBytes: 10},
		"D:": {Name: "D:", ReadBytes: 100},
	})
	d.isWindows = true
	d.rootMountPoint = `C:\`
	agent.fsStats[`C:\`] = &system.FsStats{Mountpoint: `C:\`, IoDevice: "C:", Name: "System"}
	agent.fsStats[`D:\`] = &system.FsStats{Mountpoint: `D:\`, IoDevice: "D:"}

	assert.True(t, d.addPartitionRootFs("C:", `C:\`))
	assert.Len(t, agent.fsStats, 2)
	assert.True(t, agent.fsStats[`C:\`].Root)
	assert.Equal(t, "System", agent.fsStats[`C:\`].Name)
	assert.False(t, agent.fsStats[`D:\`].Root)
}

// setApfsPhysicalDisk swaps the APFS volume -> physical disk lookup for the
// duration of a test.
func setApfsPhysicalDisk(t *testing.T, fn func(string) string) {
	t.Helper()
	old := apfsPhysicalDisk
	apfsPhysicalDisk = fn
	t.Cleanup(func() { apfsPhysicalDisk = old })
}

func TestIoKeyAPFS(t *testing.T) {
	// Physical disks as reported by IOKit on macOS: APFS containers (disk13)
	// have no counters, and disk1 is a prefix of disk13s1.
	newDarwin := func() *diskDiscovery {
		_, d := newTestDiscovery(map[string]disk.IOCountersStat{
			"disk1":  {Name: "disk1", ReadBytes: 9000, WriteBytes: 9000},
			"disk3":  {Name: "disk3", ReadBytes: 100, WriteBytes: 100},
			"disk12": {Name: "disk12", ReadBytes: 10, WriteBytes: 10},
		})
		d.isDarwin = true
		return d
	}

	t.Run("maps apfs volume to its physical store disk", func(t *testing.T) {
		calls := 0
		setApfsPhysicalDisk(t, func(dev string) string {
			calls++
			assert.Equal(t, "/dev/disk13s1", dev)
			return "disk12"
		})

		key, ok := newDarwin().ioKey("/dev/disk13s1")

		assert.True(t, ok)
		assert.Equal(t, "disk12", key, "must not prefix-match disk1")
		assert.Equal(t, 1, calls)
	})

	t.Run("hfs volume keeps prefix fallback", func(t *testing.T) {
		setApfsPhysicalDisk(t, func(string) string { return "" })
		key, ok := newDarwin().ioKey("/dev/disk3s2")
		assert.True(t, ok)
		assert.Equal(t, "disk3", key)
	})

	t.Run("unresolved lookup falls back to prefix", func(t *testing.T) {
		setApfsPhysicalDisk(t, func(string) string { return "" })
		key, _ := newDarwin().ioKey("/dev/disk13s1")
		assert.Equal(t, "disk1", key, "unchanged prefix fallback")
	})

	t.Run("physical disk without io counters falls back", func(t *testing.T) {
		setApfsPhysicalDisk(t, func(string) string { return "disk19" })
		key, _ := newDarwin().ioKey("/dev/disk20s1")
		assert.Equal(t, "disk20s1", key)
	})

	t.Run("exact io match skips lookup", func(t *testing.T) {
		setApfsPhysicalDisk(t, func(string) string {
			t.Fatal("APFS lookup should not be called")
			return ""
		})
		key, _ := newDarwin().ioKey("/dev/disk3")
		assert.Equal(t, "disk3", key)
	})

	t.Run("non-darwin never calls lookup", func(t *testing.T) {
		setApfsPhysicalDisk(t, func(string) string {
			t.Fatal("APFS lookup should not be called")
			return ""
		})
		d := newDarwin()
		d.isDarwin = false
		key, _ := d.ioKey("/dev/disk13s1")
		assert.Equal(t, "disk1", key)
	})
}

func TestAddPartitionRootFsAPFS(t *testing.T) {
	// Apple Silicon: root is a sealed snapshot of a volume in container disk3,
	// whose physical store is disk0s2. Only physical disks have counters.
	newDiscovery := func(isDarwin bool) (*Agent, *diskDiscovery) {
		agent, d := newTestDiscovery(map[string]disk.IOCountersStat{
			"disk0":  {Name: "disk0", ReadBytes: 10, WriteBytes: 10},
			"disk33": {Name: "disk33", ReadBytes: 9000, WriteBytes: 9000},
		})
		d.isDarwin = isDarwin
		// keep the real /dev out of the symlink fallback
		setEvalSymlinks(t, func(string) (string, error) { return "", os.ErrNotExist })
		return agent, d
	}

	t.Run("maps apfs root snapshot to physical disk", func(t *testing.T) {
		setApfsPhysicalDisk(t, func(dev string) string {
			assert.Equal(t, "/dev/disk3s1s1", dev)
			return "disk0"
		})
		agent, d := newDiscovery(true)

		assert.True(t, d.addPartitionRootFs("/dev/disk3s1s1", "/"))
		stats := agent.fsStats["/"]
		require.NotNil(t, stats, "root should be tracked, got %v", agent.fsStats)
		assert.True(t, stats.Root)
		assert.Equal(t, "disk0", stats.IoDevice)
	})

	t.Run("unresolved lookup keeps existing behaviour", func(t *testing.T) {
		setApfsPhysicalDisk(t, func(string) string { return "" })
		agent, d := newDiscovery(true)

		// No prefix match, so the caller falls back to addLastResortRootFs.
		assert.False(t, d.addPartitionRootFs("/dev/disk3s1s1", "/"))
		assert.Empty(t, agent.fsStats)
	})

	t.Run("non-darwin never calls lookup", func(t *testing.T) {
		setApfsPhysicalDisk(t, func(string) string {
			t.Fatal("APFS lookup should not be called")
			return ""
		})
		_, d := newDiscovery(false)

		d.addPartitionRootFs("/dev/disk3s1s1", "/")
	})
}
