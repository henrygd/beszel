//go:build testing && linux

package zfs

import (
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

func TestPoolStatsSkipsZpoolWhenDevZfsMissing(t *testing.T) {
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
		called = true
		assert.Equal(t, "zpool", name)
		assert.Equal(t, []string{"list", "-Hp", "-o", "name,size,alloc,free,health"}, args)
		return []byte("tank\t100\t50\t50\tONLINE\n"), nil
	}
	t.Cleanup(func() { commandOutput = oldCommandOutput })

	pools, err := PoolStats()
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, []PoolStat{{Name: "tank", Size: 100, Alloc: 50, Free: 50, Health: "ONLINE"}}, pools)
}
