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
	require.NoError(t, os.WriteFile(filepath.Join(poolDir, "iostats"), []byte(
		"34 1 0x01 26 7072 0 0\n"+
			"name type data\n"+
			"arc_read_bytes 4 1000\n"+
			"arc_write_bytes 4 2000\n"+
			"direct_read_bytes 4 300\n"+
			"direct_write_bytes 4 400\n",
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

func TestReadPoolIOStatsRequiresAllCounters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iostats")
	require.NoError(t, os.WriteFile(path, []byte("arc_read_bytes 4 10\n"), 0o644))
	_, _, err := readPoolIOStats(path)
	require.Error(t, err)
}
