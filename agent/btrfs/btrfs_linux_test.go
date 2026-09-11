//go:build testing && linux

package btrfs

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestFilesystems(t *testing.T) {
	root := t.TempDir()
	oldSysfs, oldMounts := sysfsPath, mountsPath
	sysfsPath, mountsPath = root, filepath.Join(root, "mounts")
	t.Cleanup(func() { sysfsPath, mountsPath = oldSysfs, oldMounts })

	fsDir := filepath.Join(root, "1b2c3d4e-0000-0000-0000-000000000000")
	write := func(rel, content string) {
		path := filepath.Join(fsDir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "features"), 0o755))
	oldUsage := filesystemUsage
	filesystemUsage = func(string) (uint64, uint64, error) { return 0, 0, os.ErrNotExist }
	t.Cleanup(func() { filesystemUsage = oldUsage })
	oldDeviceSize := deviceSize
	t.Cleanup(func() { deviceSize = oldDeviceSize })
	deviceSize = func(_ string, devid uint64) (uint64, error) {
		value, _ := utils.ReadUintFile(filepath.Join(fsDir, "recorded-size", strconv.FormatUint(devid, 10)))
		return value, nil
	}
	// Recorded member capacities differ from the unchanged backing devices.
	write("recorded-size/1", "256000\n")
	write("recorded-size/2", "128000\n")
	write("label", "tank\n")
	write("allocation/data/disk_used", "4096\n")
	write("allocation/metadata/disk_used", "2048\n")
	write("allocation/system/disk_used", "1024\n")
	write("devices/sda/size", "1000\n")
	write("devices/sda/stat", "10 0 200 0 20 0 400 0 0 0 0\n")
	write("devices/sdb/size", "1000\n")
	write("devices/sdb/stat", "10 0 100 0 20 0 100 0 0 0 0\n")
	write("devinfo/1/missing", "0\n")
	write("devinfo/1/error_stats", "write_errs 1\nread_errs 2\nflush_errs 0\ncorruption_errs 3\ngeneration_errs 0\n")
	write("devinfo/2/missing", "1\n")

	filesystems, err := Filesystems()
	require.NoError(t, err)
	require.Len(t, filesystems, 1)
	assert.Equal(t, Filesystem{
		UUID: "1b2c3d4e-0000-0000-0000-000000000000", Raw: true, Name: "tank", Size: 384000, Alloc: 7168, Health: "DEGRADED", NRead: 153600, NWrite: 256000,
		Devices: []Device{
			{Name: "devid 1", State: "ONLINE", ReadErrs: 2, WriteErrs: 1, CorruptionErrs: 3},
			{Name: "devid 2", State: "MISSING"},
		},
	}, filesystems[0])

	// Unlabeled filesystems fall back to the first mountpoint, then the UUID.
	write("label", "\n")
	require.NoError(t, os.WriteFile(mountsPath, []byte(
		"/dev/sdz1 /other btrfs rw 0 0\n/dev/sdb /mnt/storage btrfs rw 0 0\n/dev/sdb /mnt/storage/sub btrfs rw,subvol=/sub 0 0\n",
	), 0o644))
	filesystems, err = Filesystems()
	require.NoError(t, err)
	assert.Equal(t, "/mnt/storage", filesystems[0].Name)

	require.NoError(t, os.Remove(mountsPath))
	filesystems, err = Filesystems()
	require.NoError(t, err)
	assert.Equal(t, "1b2c3d4e-0000-0000-0000-000000000000", filesystems[0].Name)
	write("devinfo/3/replace_target", "1\n")
	write("recorded-size/3", "512000\n")
	filesystems, err = Filesystems()
	require.NoError(t, err)
	assert.Equal(t, uint64(384000), filesystems[0].Size, "replacement target must not inflate capacity")

	deviceSize = func(string, uint64) (uint64, error) { return 0, os.ErrPermission }
	filesystems, err = Filesystems()
	require.NoError(t, err)
	require.Len(t, filesystems, 1)
	assert.Equal(t, uint64(1024000), filesystems[0].Size)
	assert.Equal(t, "DEGRADED", filesystems[0].Health)
	assert.Equal(t, uint64(153600), filesystems[0].NRead)

	// A partial ioctl result must not be mixed with the backing-device total.
	deviceSize = func(_ string, devid uint64) (uint64, error) {
		if devid == 2 {
			return 0, os.ErrPermission
		}
		return 256000, nil
	}
	filesystems, err = Filesystems()
	require.NoError(t, err)
	assert.Equal(t, uint64(1024000), filesystems[0].Size)

	// With no mount visible (e.g. Docker), the real lookup falls back too.
	deviceSize = ioctlDeviceSize
	filesystems, err = Filesystems()
	require.NoError(t, err)
	require.Len(t, filesystems, 1)
	assert.Equal(t, uint64(1024000), filesystems[0].Size)

	filesystemUsage = func(string) (uint64, uint64, error) { return 100, 900, nil }
	filesystems, err = Filesystems()
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), filesystems[0].Size)
	assert.Equal(t, uint64(100), filesystems[0].Alloc)
	assert.False(t, filesystems[0].Raw)
}

func TestFilesystemsNoBtrfs(t *testing.T) {
	oldPath := sysfsPath
	sysfsPath = filepath.Join(t.TempDir(), "missing")
	t.Cleanup(func() { sysfsPath = oldPath })

	filesystems, err := Filesystems()
	require.NoError(t, err)
	assert.Nil(t, filesystems)
}

func TestIoctlDeviceSizeFailure(t *testing.T) {
	_, err := ioctlDeviceSize("", 1)
	require.Error(t, err)
	_, err = ioctlDeviceSize(t.TempDir(), 1)
	require.Error(t, err)
	assert.ErrorIs(t, err, unix.ENOTTY)
}

func TestMountpointsDecodeEscapes(t *testing.T) {
	oldMounts := mountsPath
	mountsPath = filepath.Join(t.TempDir(), "mounts")
	t.Cleanup(func() { mountsPath = oldMounts })
	require.NoError(t, os.WriteFile(mountsPath, []byte("/dev/test-btrfs /mnt/my\\040data btrfs rw 0 0\n"), 0o644))
	assert.Equal(t, "/mnt/my data", mountpointsByDevice()["test-btrfs"])
}

func TestFilesystemWithoutDevinfo(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "devices", "sda"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "devices", "sda", "size"), []byte("1000"), 0644))
	fs, err := readFilesystem(root, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(512000), fs.Size)
	assert.True(t, fs.Raw)
	assert.Equal(t, "UNKNOWN", fs.Health)
	assert.Empty(t, fs.Devices)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "devinfo", "1"), 0755))
	fs, err = readFilesystem(root, nil)
	require.NoError(t, err)
	assert.Equal(t, "UNKNOWN", fs.Health)
	require.Len(t, fs.Devices, 1)
	assert.Equal(t, "UNKNOWN", fs.Devices[0].State)

	// Some older interfaces lack the devices directory too.
	fs, err = readFilesystem(t.TempDir(), nil)
	require.NoError(t, err)
	assert.Equal(t, "UNKNOWN", fs.Health)
}

func TestLocalBtrfsUsage(t *testing.T) {
	path := os.Getenv("BESZEL_TEST_BTRFS_MOUNT")
	if path == "" {
		t.Skip("set BESZEL_TEST_BTRFS_MOUNT for read-only live validation")
	}
	used, available, err := statfsUsage(path)
	require.NoError(t, err)
	filesystems, err := Filesystems()
	require.NoError(t, err)
	for _, fs := range filesystems {
		if !fs.Raw && fs.Alloc == used && fs.Size == used+available {
			t.Logf("pool=%s used=%d available=%d effective_capacity=%d", fs.Name, used, available, fs.Size)
			return
		}
	}
	t.Fatal("collector did not report the mounted filesystem's usable capacity")
}

func TestMountID(t *testing.T) {
	assert.Empty(t, MountID(""))
	assert.Empty(t, MountID(filepath.Join(t.TempDir(), "missing")))
	path := os.Getenv("BESZEL_TEST_BTRFS_MOUNT")
	if path == "" {
		t.Skip("set BESZEL_TEST_BTRFS_MOUNT for live identity validation")
	}
	id := MountID(path)
	require.NotEmpty(t, id)
	assert.Equal(t, id, MountID(filepath.Join(path, ".")))
}

func TestMountinfoUUIDLookup(t *testing.T) {
	info := `1 0 0:40 /@ /inaccessible ro shared:1 - btrfs /dev/mapper/unavailable rw
2 0 0:40 /@/docker/hosts /etc/hosts ro - btrfs /dev/mapper/unavailable rw
3 0 0:40 /@/docker/hostname /etc/hostname ro - btrfs /dev/mapper/unavailable rw
4 0 0:41 /subvol /extra-filesystems/my\040disk ro master:2 - btrfs /dev/missing rw
5 0 0:42 / /ext4 ro - ext4 /dev/mapper/unavailable rw
malformed
6 0 0:43 / /bad ro - btrfs
`
	var calls []string
	mounts := mountpointsByUUID(info, func(path string) string {
		calls = append(calls, path)
		switch path {
		case "/etc/hosts":
			return "root-uuid"
		case "/extra-filesystems/my disk":
			return "extra-uuid"
		}
		return ""
	})
	assert.Equal(t, map[string]string{"uuid:root-uuid": "/etc/hosts", "uuid:extra-uuid": "/extra-filesystems/my disk"}, mounts)
	assert.Equal(t, []string{"/inaccessible", "/etc/hosts", "/extra-filesystems/my disk"}, calls)
}

func TestDockerFilesystemWithoutDeviceNodes(t *testing.T) {
	root := t.TempDir()
	oldSysfs, oldMounts, oldInfo, oldUUID, oldUsage := sysfsPath, mountsPath, mountinfoPath, mountUUID, filesystemUsage
	t.Cleanup(func() {
		sysfsPath, mountsPath, mountinfoPath, mountUUID, filesystemUsage = oldSysfs, oldMounts, oldInfo, oldUUID, oldUsage
	})
	sysfsPath = filepath.Join(root, "sysfs")
	mountsPath = filepath.Join(root, "missing-mounts")
	mountinfoPath = filepath.Join(root, "mountinfo")
	uuid := "11111111-1111-4111-8111-111111111111"
	dir := filepath.Join(sysfsPath, uuid)
	for path, content := range map[string]string{"devices/dm-0/size": "1000", "devinfo/1/missing": "0"} {
		target := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
		require.NoError(t, os.WriteFile(target, []byte(content), 0644))
	}
	require.NoError(t, os.WriteFile(mountinfoPath, []byte("2 1 0:40 /@/docker/hosts /etc/hosts ro - btrfs /dev/mapper/not-in-container rw\n"), 0644))
	mountUUID = func(path string) string {
		if path == "/etc/hosts" {
			return uuid
		}
		return ""
	}
	filesystemUsage = func(path string) (uint64, uint64, error) { require.Equal(t, "/etc/hosts", path); return 100, 900, nil }
	fs, err := Filesystems()
	require.NoError(t, err)
	require.Len(t, fs, 1)
	assert.Equal(t, uuid, fs[0].MountID)
	assert.Equal(t, "dm-0", fs[0].IODevice)
	assert.False(t, fs[0].Raw)
	assert.Equal(t, uint64(1000), fs[0].Size)
}

func TestLivePoolMountIdentity(t *testing.T) {
	path := os.Getenv("BESZEL_TEST_BTRFS_MOUNT")
	if path == "" {
		t.Skip("set BESZEL_TEST_BTRFS_MOUNT for live validation")
	}
	id := MountID(path)
	require.NotEmpty(t, id)
	pools, err := Filesystems()
	require.NoError(t, err)
	for _, pool := range pools {
		if pool.UUID != id {
			continue
		}
		assert.Equal(t, id, pool.MountID)
		assert.False(t, pool.Raw)
		t.Logf("uuid=%s mount_identity=%s io_device=%s raw=%v", pool.UUID, pool.MountID, pool.IODevice, pool.Raw)
		return
	}
	t.Fatal("mounted Btrfs filesystem was not discovered")
}
