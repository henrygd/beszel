//go:build testing && linux

package btrfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		Name: "tank", Size: 1024000, Alloc: 7168, Health: "DEGRADED", NRead: 153600, NWrite: 256000,
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
}

func TestFilesystemsNoBtrfs(t *testing.T) {
	oldPath := sysfsPath
	sysfsPath = filepath.Join(t.TempDir(), "missing")
	t.Cleanup(func() { sysfsPath = oldPath })

	filesystems, err := Filesystems()
	require.NoError(t, err)
	assert.Nil(t, filesystems)
}
