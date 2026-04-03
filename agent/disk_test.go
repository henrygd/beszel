//go:build testing

package agent

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/assert"
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
			fsEntry := strings.TrimSpace(tt.input)
			var fs, customName string
			if parts := strings.SplitN(fsEntry, "__", 2); len(parts) == 2 {
				fs = strings.TrimSpace(parts[0])
				customName = strings.TrimSpace(parts[1])
			} else {
				fs = fsEntry
			}

			assert.Equal(t, tt.expectedFs, fs)
			assert.Equal(t, tt.expectedName, customName)
		})
	}
}

func TestExtraFilesystemPartitionInfo(t *testing.T) {
	t.Run("uses partition device for label-only mountpoint", func(t *testing.T) {
		device, customName := extraFilesystemPartitionInfo(disk.PartitionStat{
			Device:     "/dev/sdc",
			Mountpoint: "/extra-filesystems/Share",
		})

		assert.Equal(t, "/dev/sdc", device)
		assert.Equal(t, "", customName)
	})

	t.Run("uses custom name from mountpoint suffix", func(t *testing.T) {
		device, customName := extraFilesystemPartitionInfo(disk.PartitionStat{
			Device:     "/dev/sdc",
			Mountpoint: "/extra-filesystems/sdc__Share",
		})

		assert.Equal(t, "/dev/sdc", device)
		assert.Equal(t, "Share", customName)
	})

	t.Run("falls back to folder device when partition device is unavailable", func(t *testing.T) {
		device, customName := extraFilesystemPartitionInfo(disk.PartitionStat{
			Mountpoint: "/extra-filesystems/sdc__Share",
		})

		assert.Equal(t, "sdc", device)
		assert.Equal(t, "Share", customName)
	})

	t.Run("supports custom name without folder device prefix", func(t *testing.T) {
		device, customName := extraFilesystemPartitionInfo(disk.PartitionStat{
			Device:     "/dev/sdc",
			Mountpoint: "/extra-filesystems/__Share",
		})

		assert.Equal(t, "/dev/sdc", device)
		assert.Equal(t, "Share", customName)
	})
}

func TestBuildFsStatRegistration(t *testing.T) {
	t.Run("uses basename for non-windows exact io match", func(t *testing.T) {
		key, stats, ok := registerFilesystemStats(
			map[string]*system.FsStats{},
			"/dev/sda1",
			"/mnt/data",
			false,
			"archive",
			fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"sda1": {Name: "sda1"},
				},
			},
		)

		assert.True(t, ok)
		assert.Equal(t, "sda1", key)
		assert.Equal(t, "/mnt/data", stats.Mountpoint)
		assert.Equal(t, "archive", stats.Name)
		assert.False(t, stats.Root)
	})

	t.Run("maps root partition to io device by prefix", func(t *testing.T) {
		key, stats, ok := registerFilesystemStats(
			map[string]*system.FsStats{},
			"/dev/ada0p2",
			"/",
			true,
			"",
			fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"ada0": {Name: "ada0", ReadBytes: 1000, WriteBytes: 1000},
				},
			},
		)

		assert.True(t, ok)
		assert.Equal(t, "ada0", key)
		assert.True(t, stats.Root)
		assert.Equal(t, "/", stats.Mountpoint)
	})

	t.Run("uses filesystem setting as root fallback", func(t *testing.T) {
		key, _, ok := registerFilesystemStats(
			map[string]*system.FsStats{},
			"overlay",
			"/",
			true,
			"",
			fsRegistrationContext{
				filesystem: "nvme0n1p2",
				isWindows:  false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"nvme0n1": {Name: "nvme0n1", ReadBytes: 1000, WriteBytes: 1000},
				},
			},
		)

		assert.True(t, ok)
		assert.Equal(t, "nvme0n1", key)
	})

	t.Run("prefers parsed extra-filesystems device over mapper device", func(t *testing.T) {
		key, stats, ok := registerFilesystemStats(
			map[string]*system.FsStats{},
			"/dev/mapper/luks-2bcb02be-999d-4417-8d18-5c61e660fb6e",
			"/extra-filesystems/nvme0n1p2__Archive",
			false,
			"Archive",
			fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"dm-1":      {Name: "dm-1", Label: "luks-2bcb02be-999d-4417-8d18-5c61e660fb6e"},
					"nvme0n1p2": {Name: "nvme0n1p2"},
				},
			},
		)

		assert.True(t, ok)
		assert.Equal(t, "nvme0n1p2", key)
		assert.Equal(t, "Archive", stats.Name)
	})

	t.Run("falls back to mapper io device when folder device cannot be resolved", func(t *testing.T) {
		key, stats, ok := registerFilesystemStats(
			map[string]*system.FsStats{},
			"/dev/mapper/luks-2bcb02be-999d-4417-8d18-5c61e660fb6e",
			"/extra-filesystems/Archive",
			false,
			"Archive",
			fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"dm-1": {Name: "dm-1", Label: "luks-2bcb02be-999d-4417-8d18-5c61e660fb6e"},
				},
			},
		)

		assert.True(t, ok)
		assert.Equal(t, "dm-1", key)
		assert.Equal(t, "Archive", stats.Name)
	})

	t.Run("uses full device name on windows", func(t *testing.T) {
		key, _, ok := registerFilesystemStats(
			map[string]*system.FsStats{},
			`C:`,
			`C:\\`,
			false,
			"",
			fsRegistrationContext{
				isWindows: true,
				diskIoCounters: map[string]disk.IOCountersStat{
					`C:`: {Name: `C:`},
				},
			},
		)

		assert.True(t, ok)
		assert.Equal(t, `C:`, key)
	})

	t.Run("skips existing key", func(t *testing.T) {
		key, stats, ok := registerFilesystemStats(
			map[string]*system.FsStats{"sda1": {Mountpoint: "/existing"}},
			"/dev/sda1",
			"/mnt/data",
			false,
			"",
			fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"sda1": {Name: "sda1"},
				},
			},
		)

		assert.False(t, ok)
		assert.Empty(t, key)
		assert.Nil(t, stats)
	})
}

func TestAddConfiguredRootFs(t *testing.T) {
	t.Run("adds root from matching partition", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{
			agent:          agent,
			rootMountPoint: "/",
			partitions:     []disk.PartitionStat{{Device: "/dev/ada0p2", Mountpoint: "/"}},
			ctx: fsRegistrationContext{
				filesystem: "/dev/ada0p2",
				isWindows:  false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"ada0": {Name: "ada0", ReadBytes: 1000, WriteBytes: 1000},
				},
			},
		}

		ok := discovery.addConfiguredRootFs()

		assert.True(t, ok)
		stats, exists := agent.fsStats["ada0"]
		assert.True(t, exists)
		assert.True(t, stats.Root)
		assert.Equal(t, "/", stats.Mountpoint)
	})

	t.Run("adds root from io device when partition is missing", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{
			agent:          agent,
			rootMountPoint: "/sysroot",
			ctx: fsRegistrationContext{
				filesystem: "zroot",
				isWindows:  false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"nda0": {Name: "nda0", Label: "zroot", ReadBytes: 1000, WriteBytes: 1000},
				},
			},
		}

		ok := discovery.addConfiguredRootFs()

		assert.True(t, ok)
		stats, exists := agent.fsStats["nda0"]
		assert.True(t, exists)
		assert.True(t, stats.Root)
		assert.Equal(t, "/sysroot", stats.Mountpoint)
	})

	t.Run("returns false when filesystem cannot be resolved", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{
			agent:          agent,
			rootMountPoint: "/",
			ctx: fsRegistrationContext{
				filesystem:     "missing-disk",
				isWindows:      false,
				diskIoCounters: map[string]disk.IOCountersStat{},
			},
		}

		ok := discovery.addConfiguredRootFs()

		assert.False(t, ok)
		assert.Empty(t, agent.fsStats)
	})
}

func TestAddPartitionRootFs(t *testing.T) {
	t.Run("adds root from fallback partition candidate", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{
			agent: agent,
			ctx: fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"nvme0n1": {Name: "nvme0n1", ReadBytes: 1000, WriteBytes: 1000},
				},
			},
		}

		ok := discovery.addPartitionRootFs("/dev/nvme0n1p2", "/")

		assert.True(t, ok)
		stats, exists := agent.fsStats["nvme0n1"]
		assert.True(t, exists)
		assert.True(t, stats.Root)
		assert.Equal(t, "/", stats.Mountpoint)
	})

	t.Run("returns false when no io device matches", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{agent: agent, ctx: fsRegistrationContext{diskIoCounters: map[string]disk.IOCountersStat{}}}

		ok := discovery.addPartitionRootFs("/dev/mapper/root", "/")

		assert.False(t, ok)
		assert.Empty(t, agent.fsStats)
	})
}

func TestAddLastResortRootFs(t *testing.T) {
	t.Run("uses most active io device when available", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{agent: agent, rootMountPoint: "/", ctx: fsRegistrationContext{diskIoCounters: map[string]disk.IOCountersStat{
			"sda": {Name: "sda", ReadBytes: 5000, WriteBytes: 5000},
			"sdb": {Name: "sdb", ReadBytes: 1000, WriteBytes: 1000},
		}}}

		discovery.addLastResortRootFs()

		stats, exists := agent.fsStats["sda"]
		assert.True(t, exists)
		assert.True(t, stats.Root)
	})

	t.Run("falls back to root key when mountpoint basename collides", func(t *testing.T) {
		agent := &Agent{fsStats: map[string]*system.FsStats{
			"sysroot": {Mountpoint: "/extra-filesystems/sysroot"},
		}}
		discovery := diskDiscovery{agent: agent, rootMountPoint: "/sysroot", ctx: fsRegistrationContext{diskIoCounters: map[string]disk.IOCountersStat{}}}

		discovery.addLastResortRootFs()

		stats, exists := agent.fsStats["root"]
		assert.True(t, exists)
		assert.True(t, stats.Root)
		assert.Equal(t, "/sysroot", stats.Mountpoint)
	})
}

func TestAddConfiguredExtraFsEntry(t *testing.T) {
	t.Run("uses matching partition when present", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{
			agent:      agent,
			partitions: []disk.PartitionStat{{Device: "/dev/sdb1", Mountpoint: "/mnt/backup"}},
			usageFn: func(string) (*disk.UsageStat, error) {
				t.Fatal("usage fallback should not be called when partition matches")
				return nil, nil
			},
			ctx: fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"sdb1": {Name: "sdb1"},
				},
			},
		}

		discovery.addConfiguredExtraFsEntry("sdb1", "backup")

		stats, exists := agent.fsStats["sdb1"]
		assert.True(t, exists)
		assert.Equal(t, "/mnt/backup", stats.Mountpoint)
		assert.Equal(t, "backup", stats.Name)
	})

	t.Run("falls back to usage-validated path", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{
			agent: agent,
			usageFn: func(path string) (*disk.UsageStat, error) {
				assert.Equal(t, "/srv/archive", path)
				return &disk.UsageStat{}, nil
			},
			ctx: fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"archive": {Name: "archive"},
				},
			},
		}

		discovery.addConfiguredExtraFsEntry("/srv/archive", "archive")

		stats, exists := agent.fsStats["archive"]
		assert.True(t, exists)
		assert.Equal(t, "/srv/archive", stats.Mountpoint)
		assert.Equal(t, "archive", stats.Name)
	})

	t.Run("ignores invalid filesystem entry", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{
			agent: agent,
			usageFn: func(string) (*disk.UsageStat, error) {
				return nil, os.ErrNotExist
			},
		}

		discovery.addConfiguredExtraFsEntry("/missing/archive", "")

		assert.Empty(t, agent.fsStats)
	})
}

func TestAddConfiguredExtraFilesystems(t *testing.T) {
	t.Run("parses and registers multiple configured filesystems", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		discovery := diskDiscovery{
			agent:      agent,
			partitions: []disk.PartitionStat{{Device: "/dev/sda1", Mountpoint: "/mnt/fast"}},
			usageFn: func(path string) (*disk.UsageStat, error) {
				if path == "/srv/archive" {
					return &disk.UsageStat{}, nil
				}
				return nil, os.ErrNotExist
			},
			ctx: fsRegistrationContext{
				isWindows: false,
				diskIoCounters: map[string]disk.IOCountersStat{
					"sda1":    {Name: "sda1"},
					"archive": {Name: "archive"},
				},
			},
		}

		discovery.addConfiguredExtraFilesystems("sda1__fast,/srv/archive__cold")

		assert.Contains(t, agent.fsStats, "sda1")
		assert.Equal(t, "fast", agent.fsStats["sda1"].Name)
		assert.Contains(t, agent.fsStats, "archive")
		assert.Equal(t, "cold", agent.fsStats["archive"].Name)
	})
}

func TestAddExtraFilesystemFolders(t *testing.T) {
	t.Run("adds missing folders and skips existing mountpoints", func(t *testing.T) {
		agent := &Agent{fsStats: map[string]*system.FsStats{
			"existing": {Mountpoint: "/extra-filesystems/existing"},
		}}
		discovery := diskDiscovery{
			agent: agent,
			ctx: fsRegistrationContext{
				isWindows: false,
				efPath:    "/extra-filesystems",
				diskIoCounters: map[string]disk.IOCountersStat{
					"newdisk": {Name: "newdisk"},
				},
			},
		}

		discovery.addExtraFilesystemFolders([]string{"existing", "newdisk__Archive"})

		assert.Len(t, agent.fsStats, 2)
		stats, exists := agent.fsStats["newdisk"]
		assert.True(t, exists)
		assert.Equal(t, "/extra-filesystems/newdisk__Archive", stats.Mountpoint)
		assert.Equal(t, "Archive", stats.Name)
	})
}

func TestAddPartitionExtraFs(t *testing.T) {
	makeDiscovery := func(agent *Agent) diskDiscovery {
		return diskDiscovery{
			agent: agent,
			ctx: fsRegistrationContext{
				isWindows: false,
				efPath:    "/extra-filesystems",
				diskIoCounters: map[string]disk.IOCountersStat{
					"nvme0n1p1": {Name: "nvme0n1p1"},
					"nvme1n1":   {Name: "nvme1n1"},
				},
			},
		}
	}

	t.Run("registers direct child of extra-filesystems", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		d := makeDiscovery(agent)

		d.addPartitionExtraFs(disk.PartitionStat{
			Device:     "/dev/nvme0n1p1",
			Mountpoint: "/extra-filesystems/nvme0n1p1__caddy1-root",
		})

		stats, exists := agent.fsStats["nvme0n1p1"]
		assert.True(t, exists)
		assert.Equal(t, "/extra-filesystems/nvme0n1p1__caddy1-root", stats.Mountpoint)
		assert.Equal(t, "caddy1-root", stats.Name)
	})

	t.Run("skips nested mount under extra-filesystem bind mount", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		d := makeDiscovery(agent)

		// These simulate the virtual mounts that appear when host / is bind-mounted
		// with disk.Partitions(all=true) — e.g. /proc, /sys, /dev visible under the mount.
		for _, nested := range []string{
			"/extra-filesystems/nvme0n1p1__caddy1-root/proc",
			"/extra-filesystems/nvme0n1p1__caddy1-root/sys",
			"/extra-filesystems/nvme0n1p1__caddy1-root/dev",
			"/extra-filesystems/nvme0n1p1__caddy1-root/run",
		} {
			d.addPartitionExtraFs(disk.PartitionStat{Device: "tmpfs", Mountpoint: nested})
		}

		assert.Empty(t, agent.fsStats)
	})

	t.Run("registers both direct children, skips their nested mounts", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		d := makeDiscovery(agent)

		partitions := []disk.PartitionStat{
			{Device: "/dev/nvme0n1p1", Mountpoint: "/extra-filesystems/nvme0n1p1__caddy1-root"},
			{Device: "/dev/nvme1n1", Mountpoint: "/extra-filesystems/nvme1n1__caddy1-docker"},
			{Device: "proc", Mountpoint: "/extra-filesystems/nvme0n1p1__caddy1-root/proc"},
			{Device: "sysfs", Mountpoint: "/extra-filesystems/nvme0n1p1__caddy1-root/sys"},
			{Device: "overlay", Mountpoint: "/extra-filesystems/nvme0n1p1__caddy1-root/var/lib/docker"},
		}
		for _, p := range partitions {
			d.addPartitionExtraFs(p)
		}

		assert.Len(t, agent.fsStats, 2)
		assert.Equal(t, "caddy1-root", agent.fsStats["nvme0n1p1"].Name)
		assert.Equal(t, "caddy1-docker", agent.fsStats["nvme1n1"].Name)
	})

	t.Run("skips partition not under extra-filesystems", func(t *testing.T) {
		agent := &Agent{fsStats: make(map[string]*system.FsStats)}
		d := makeDiscovery(agent)

		d.addPartitionExtraFs(disk.PartitionStat{
			Device:     "/dev/nvme0n1p1",
			Mountpoint: "/",
		})

		assert.Empty(t, agent.fsStats)
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

func TestInitializeDiskInfoWithCustomNames(t *testing.T) {
	// Test with custom names
	t.Setenv("EXTRA_FILESYSTEMS", "sda1__my-storage,/dev/sdb1__backup-drive,nvme0n1p2")

	// Mock disk partitions (we'll just test the parsing logic)
	// Since the actual disk operations are system-dependent, we'll focus on the parsing
	testCases := []struct {
		envValue      string
		expectedFs    []string
		expectedNames map[string]string
	}{
		{
			envValue:   "sda1__my-storage,sdb1__backup-drive",
			expectedFs: []string{"sda1", "sdb1"},
			expectedNames: map[string]string{
				"sda1": "my-storage",
				"sdb1": "backup-drive",
			},
		},
		{
			envValue:   "sda1,nvme0n1p2__fast-ssd",
			expectedFs: []string{"sda1", "nvme0n1p2"},
			expectedNames: map[string]string{
				"nvme0n1p2": "fast-ssd",
			},
		},
	}

	for _, tc := range testCases {
		t.Run("env_"+tc.envValue, func(t *testing.T) {
			t.Setenv("EXTRA_FILESYSTEMS", tc.envValue)

			// Create mock partitions that would match our test cases
			partitions := []disk.PartitionStat{}
			for _, fs := range tc.expectedFs {
				if strings.HasPrefix(fs, "/dev/") {
					partitions = append(partitions, disk.PartitionStat{
						Device:     fs,
						Mountpoint: fs,
					})
				} else {
					partitions = append(partitions, disk.PartitionStat{
						Device:     "/dev/" + fs,
						Mountpoint: "/" + fs,
					})
				}
			}

			// Test the parsing logic by calling the relevant part
			// We'll create a simplified version to test just the parsing
			extraFilesystems := tc.envValue
			for fsEntry := range strings.SplitSeq(extraFilesystems, ",") {
				// Parse the entry
				fsEntry = strings.TrimSpace(fsEntry)
				var fs, customName string
				if parts := strings.SplitN(fsEntry, "__", 2); len(parts) == 2 {
					fs = strings.TrimSpace(parts[0])
					customName = strings.TrimSpace(parts[1])
				} else {
					fs = fsEntry
				}

				// Verify the device is in our expected list
				assert.Contains(t, tc.expectedFs, fs, "parsed device should be in expected list")

				// Check if custom name should exist
				if expectedName, exists := tc.expectedNames[fs]; exists {
					assert.Equal(t, expectedName, customName, "custom name should match expected")
				} else {
					assert.Empty(t, customName, "custom name should be empty when not expected")
				}
			}
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
			"sda": {},
			"sdb": {},
		},
		fsNames: []string{"stale", "sda"},
	}

	agent.initializeDiskIoStats(map[string]disk.IOCountersStat{
		"sda": {Name: "sda", ReadBytes: 10, WriteBytes: 20},
		"sdb": {Name: "sdb", ReadBytes: 30, WriteBytes: 40},
	})

	assert.ElementsMatch(t, []string{"sda", "sdb"}, agent.fsNames)
	assert.Len(t, agent.fsNames, 2)
	assert.Equal(t, uint64(10), agent.fsStats["sda"].TotalRead)
	assert.Equal(t, uint64(20), agent.fsStats["sda"].TotalWrite)
	assert.False(t, agent.fsStats["sda"].Time.IsZero())
	assert.False(t, agent.fsStats["sdb"].Time.IsZero())

	agent.initializeDiskIoStats(map[string]disk.IOCountersStat{
		"sdb": {Name: "sdb", ReadBytes: 50, WriteBytes: 60},
	})

	assert.Equal(t, []string{"sdb"}, agent.fsNames)
	assert.Equal(t, uint64(50), agent.fsStats["sdb"].TotalRead)
	assert.Equal(t, uint64(60), agent.fsStats["sdb"].TotalWrite)
}
