//go:build testing && linux

package zfs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolKernelStats(t *testing.T) {
	root := t.TempDir()
	oldPath := procZfsPath
	procZfsPath = root
	t.Cleanup(func() { procZfsPath = oldPath })

	poolDir := filepath.Join(root, "tank")
	require.NoError(t, os.MkdirAll(poolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "io"), []byte(
		"11 3 0x00 1 80 0 0\n"+
			"nread nwritten reads writes wtime wlentime wupdate rtime rlentime rupdate wcnt rcnt\n"+
			"1884160 6450688 22 978 0 0 0 0 0 0 0 0\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "state"), []byte("DEGRADED\n"), 0o644))

	stats, err := PoolKernelStats()
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, PoolKernelStat{
		Name: "tank", Health: "DEGRADED", NRead: 1884160, NWrite: 6450688,
	}, stats[0])
}

func TestPoolKernelStatsOpenZfs24(t *testing.T) {
	root := t.TempDir()
	oldPath := procZfsPath
	procZfsPath = root
	t.Cleanup(func() { procZfsPath = oldPath })

	poolDir := filepath.Join(root, "tank")
	require.NoError(t, os.MkdirAll(poolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "state"), []byte("ONLINE\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "objset-0x1"), []byte(
		"34 1 0x01 28 7872 0 0\n"+
			"name type data\n"+
			"dataset_name 7 tank\n"+
			"nwritten 4 2000\n"+
			"nread 4 1000\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "objset-0x2"), []byte(
		"34 1 0x01 28 7872 0 0\n"+
			"name type data\n"+
			"dataset_name 7 tank/videos\n"+
			"nwritten 4 400\n"+
			"nread 4 300\n",
	), 0o644))

	stats, err := PoolKernelStats()
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, PoolKernelStat{
		Name: "tank", Health: "ONLINE", NRead: 1300, NWrite: 2400,
	}, stats[0])
}

func TestPoolKernelStatsOpenZfs23(t *testing.T) {
	root := t.TempDir()
	oldPath := procZfsPath
	procZfsPath = root
	t.Cleanup(func() { procZfsPath = oldPath })

	// OpenZFS 2.3+ exposes logical pool read/write counters in "iostats".
	// They cover every objset, including mounted snapshots, which never get
	// an "objset-*" kstat, so the objset sum alone under-reports reads.
	poolDir := filepath.Join(root, "tank")
	require.NoError(t, os.MkdirAll(poolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "state"), []byte("ONLINE\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "iostats"), []byte(
		"15 1 0x01 26 5176 227423 2127661979232\n"+
			"name type data\n"+
			"trim_extents_written 4 0\n"+
			"arc_read_count 4 12\n"+
			"arc_read_bytes 4 5000\n"+
			"arc_write_count 4 7\n"+
			"arc_write_bytes 4 3000\n"+
			"direct_read_count 4 1\n"+
			"direct_read_bytes 4 500\n"+
			"direct_write_count 4 1\n"+
			"direct_write_bytes 4 200\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "objset-0x1"), []byte(
		"34 1 0x01 28 7872 0 0\n"+
			"name type data\n"+
			"dataset_name 7 tank\n"+
			"nwritten 4 2000\n"+
			"nread 4 1000\n",
	), 0o644))

	stats, err := PoolKernelStats()
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, PoolKernelStat{
		Name: "tank", Health: "ONLINE", NRead: 5500, NWrite: 3200,
	}, stats[0])
}

func TestPoolKernelStatsIOStatsTrimOnly(t *testing.T) {
	root := t.TempDir()
	oldPath := procZfsPath
	procZfsPath = root
	t.Cleanup(func() { procZfsPath = oldPath })

	// Before OpenZFS 2.3 the "iostats" file only reports TRIM counters, so
	// the per-dataset "objset-*" files remain the only usable source.
	poolDir := filepath.Join(root, "tank")
	require.NoError(t, os.MkdirAll(poolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "state"), []byte("ONLINE\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "iostats"), []byte(
		"15 1 0x01 18 3736 227423 2127661979232\n"+
			"name type data\n"+
			"trim_extents_written 4 10\n"+
			"trim_bytes_written 4 4096\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "objset-0x1"), []byte(
		"34 1 0x01 28 7872 0 0\n"+
			"name type data\n"+
			"dataset_name 7 tank\n"+
			"nwritten 4 2000\n"+
			"nread 4 1000\n",
	), 0o644))

	stats, err := PoolKernelStats()
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, PoolKernelStat{
		Name: "tank", Health: "ONLINE", NRead: 1000, NWrite: 2000,
	}, stats[0])
}

func TestPoolKernelStatsIOStatsErrorDoesNotFallback(t *testing.T) {
	root := t.TempDir()
	oldPath := procZfsPath
	procZfsPath = root
	t.Cleanup(func() { procZfsPath = oldPath })

	// A malformed "iostats" on a kernel that supports it must surface an
	// error. Silently switching to the per-dataset sum would drop snapshot
	// reads and, when "iostats" recovers, make kernelStats report a false
	// I/O spike by comparing counters from two different interfaces.
	poolDir := filepath.Join(root, "tank")
	require.NoError(t, os.MkdirAll(poolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "state"), []byte("ONLINE\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "iostats"), []byte(
		"arc_read_bytes 4 notanumber\n"+
			"arc_write_bytes 4 3000\n"+
			"direct_read_bytes 4 500\n"+
			"direct_write_bytes 4 200\n",
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "objset-0x1"), []byte(
		"34 1 0x01 28 7872 0 0\n"+
			"name type data\n"+
			"dataset_name 7 tank\n"+
			"nwritten 4 2000\n"+
			"nread 4 1000\n",
	), 0o644))

	_, err := PoolKernelStats()
	require.Error(t, err)
}

func TestPoolKernelStatsNoZfs(t *testing.T) {
	oldPath := procZfsPath
	procZfsPath = t.TempDir()
	t.Cleanup(func() { procZfsPath = oldPath })

	_, err := PoolKernelStats()
	assert.ErrorIs(t, err, ErrNoZfs)
}

func TestReadPoolIORejectsMalformedCounters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "io")
	require.NoError(t, os.WriteFile(path, []byte("nread nwritten\nnope 10\n"), 0o644))
	_, _, err := readPoolIO(path)
	require.Error(t, err)
}

func TestReadObjsetIORequiresAllCounters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "objset-0x1")
	require.NoError(t, os.WriteFile(path, []byte("nread 4 10\n"), 0o644))
	_, _, err := readObjsetIO(path)
	require.Error(t, err)
}

func TestReadPoolIOStatsRequiresAllCounters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iostats")
	require.NoError(t, os.WriteFile(path, []byte(
		"arc_read_bytes 4 10\ndirect_read_bytes 4 5\narc_write_bytes 4 7\n"), 0o644))
	_, _, err := readPoolIOStats(path)
	require.Error(t, err)
}

func TestReadPoolIOStatsTrimOnlySignalsFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iostats")
	require.NoError(t, os.WriteFile(path, []byte(
		"trim_extents_written 4 10\ntrim_bytes_written 4 4096\n"), 0o644))
	_, _, err := readPoolIOStats(path)
	assert.ErrorIs(t, err, errNoPoolIOStats)
}

func TestCollectorsSkipCommandsWhenDevZfsMissing(t *testing.T) {
	root := t.TempDir()
	oldDevZfsPath := devZfsPath
	devZfsPath = filepath.Join(root, "missing")
	t.Cleanup(func() { devZfsPath = oldDevZfsPath })

	oldCommandOutput := commandOutput
	commandOutput = func(name string, args ...string) ([]byte, error) {
		t.Fatalf("unexpected %s call with %v", name, args)
		return nil, nil
	}
	t.Cleanup(func() { commandOutput = oldCommandOutput })

	_, err := PoolStats()
	assert.ErrorIs(t, err, ErrNoZfs)
	_, err = Datasets()
	assert.ErrorIs(t, err, ErrNoZfs)
}

func TestDatasetsDelegatesWhenDevZfsPresent(t *testing.T) {
	oldDevZfsPath := devZfsPath
	devZfsPath = filepath.Join(t.TempDir(), "zfs")
	require.NoError(t, os.WriteFile(devZfsPath, nil, 0o644))
	t.Cleanup(func() { devZfsPath = oldDevZfsPath })

	oldCommandOutput := commandOutput
	commandOutput = func(name string, args ...string) ([]byte, error) {
		assert.Equal(t, "zfs", name)
		assert.Equal(t, []string{"list", "-Hp", "-o", "name,used,avail,mountpoint"}, args)
		return []byte("tank\t50\t50\t/tank\n"), nil
	}
	t.Cleanup(func() { commandOutput = oldCommandOutput })

	datasets, err := Datasets()
	require.NoError(t, err)
	assert.Equal(t, []Dataset{{Name: "tank", Used: 50, Avail: 50, Mountpoint: "/tank"}}, datasets)
}

func TestPoolStatsDelegatesToZpoolWhenDevZfsPresent(t *testing.T) {
	root := t.TempDir()
	devFile := filepath.Join(root, "zfs")
	require.NoError(t, os.WriteFile(devFile, []byte(""), 0o644))

	oldDevZfsPath := devZfsPath
	devZfsPath = devFile
	t.Cleanup(func() { devZfsPath = oldDevZfsPath })

	oldCommandOutput := commandOutput
	called := false
	commandOutput = func(name string, args ...string) ([]byte, error) {
		switch name {
		case "zpool":
			called = true
			assert.Equal(t, []string{"list", "-Hp", "-o", "name,size,alloc,free,health"}, args)
			// raidz: the raw figures include parity
			return []byte("tank\t150\t60\t90\tONLINE\n"), nil
		case "zfs":
			assert.Equal(t, []string{"list", "-Hp", "-d", "0", "-o", "name,used,avail,mountpoint"}, args)
			return []byte("tank\t40\t60\t/tank\n"), nil
		}
		t.Fatalf("unexpected %s call with %v", name, args)
		return nil, nil
	}
	t.Cleanup(func() { commandOutput = oldCommandOutput })

	pools, err := PoolStats()
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, []PoolStat{{Name: "tank", Size: 100, Alloc: 40, Free: 60, Health: "ONLINE"}}, pools)
}

func TestPoolStatsKeepsRawCapacityWithoutRootDataset(t *testing.T) {
	devFile := filepath.Join(t.TempDir(), "zfs")
	require.NoError(t, os.WriteFile(devFile, nil, 0o644))
	oldDevZfsPath := devZfsPath
	devZfsPath = devFile
	t.Cleanup(func() { devZfsPath = oldDevZfsPath })

	for _, tc := range []struct {
		name   string
		zfsOut []byte
		zfsErr error
	}{
		{"zfs list fails", nil, errors.New("zfs list failed")},
		{"root dataset missing", []byte("other\t1\t1\t/other\n"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldCommandOutput := commandOutput
			commandOutput = func(name string, args ...string) ([]byte, error) {
				if name == "zpool" {
					return []byte("tank\t150\t60\t90\tONLINE\n"), nil
				}
				return tc.zfsOut, tc.zfsErr
			}
			t.Cleanup(func() { commandOutput = oldCommandOutput })

			pools, err := PoolStats()
			require.NoError(t, err)
			assert.Equal(t, []PoolStat{{Name: "tank", Size: 150, Alloc: 60, Free: 90, Health: "ONLINE", Raw: true}}, pools)
		})
	}
}
