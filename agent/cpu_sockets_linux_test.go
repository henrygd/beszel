//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountCpuPackages(t *testing.T) {
	all := func(int) bool { return true }
	// writeCpus creates cpuN/topology/physical_package_id for each id
	writeCpus := func(t *testing.T, root string, ids ...string) {
		t.Helper()
		for i, id := range ids {
			dir := filepath.Join(root, "devices/system/cpu", "cpu"+string(rune('0'+i)), "topology")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "physical_package_id"), []byte(id+"\n"), 0o644))
		}
	}

	t.Run("dual socket with several cpus per package", func(t *testing.T) {
		root := t.TempDir()
		writeCpus(t, root, "0", "0", "1", "1", "0", "1")
		assert.Equal(t, 2, countCpuPackages(root, all))
	})

	t.Run("single socket", func(t *testing.T) {
		root := t.TempDir()
		writeCpus(t, root, "0", "0", "0", "0")
		assert.Equal(t, 1, countCpuPackages(root, all))
	})

	t.Run("ignores non-cpu directories and empty ids", func(t *testing.T) {
		root := t.TempDir()
		writeCpus(t, root, "0", "")
		for _, name := range []string{"cpufreq", "cpuidle"} {
			dir := filepath.Join(root, "devices/system/cpu", name, "topology")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "physical_package_id"), []byte("7\n"), 0o644))
		}
		assert.Equal(t, 1, countCpuPackages(root, all))
	})

	t.Run("missing sysfs", func(t *testing.T) {
		assert.Equal(t, 0, countCpuPackages(filepath.Join(t.TempDir(), "missing"), all))
	})

	t.Run("only counts cpus in the affinity mask", func(t *testing.T) {
		root := t.TempDir()
		writeCpus(t, root, "0", "0", "1", "1")
		assert.Equal(t, 1, countCpuPackages(root, func(cpu int) bool { return cpu < 2 }))
	})
}

func TestKernelAtLeast(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"6.0.0", true},
		{"6.1.0-25-arm64", true},
		{"5.10.160-rockchip", false},
		{"5.15", false},
		{"7.0.0-38-generic", true},
		{"6.0-rc1", true},
		{"", false},
		{"garbage", false},
	} {
		assert.Equal(t, tc.want, kernelAtLeast(tc.version, 6, 0), tc.version)
	}
}

func TestPackageIdsReliable(t *testing.T) {
	assert.True(t, packageIdsReliable("amd64"))
	assert.False(t, packageIdsReliable("arm64"))
	assert.False(t, packageIdsReliable("riscv64"))
}
