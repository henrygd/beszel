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
		assert.Equal(t, 2, countCpuPackages(root))
	})

	t.Run("single socket", func(t *testing.T) {
		root := t.TempDir()
		writeCpus(t, root, "0", "0", "0", "0")
		assert.Equal(t, 1, countCpuPackages(root))
	})

	t.Run("ignores non-cpu directories and empty ids", func(t *testing.T) {
		root := t.TempDir()
		writeCpus(t, root, "0", "")
		for _, name := range []string{"cpufreq", "cpuidle"} {
			dir := filepath.Join(root, "devices/system/cpu", name, "topology")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "physical_package_id"), []byte("7\n"), 0o644))
		}
		assert.Equal(t, 1, countCpuPackages(root))
	})

	t.Run("missing sysfs", func(t *testing.T) {
		assert.Equal(t, 0, countCpuPackages(filepath.Join(t.TempDir(), "missing")))
	})
}
