//go:build windows && testing

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveOutdatedLHM(t *testing.T) {
	base := t.TempDir()
	write := func(name string) {
		path := filepath.Join(base, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
	}
	// helper files extracted straight into the base dir by older releases
	write("beszel_lhm.exe")
	write("LibreHardwareMonitorLib.dll")
	write("beszel_lhm.exe.config")
	write("beszel_lhm.pdb")
	// helpers from other agent versions, and the current one
	write("lhm-0.20.0/beszel_lhm.exe")
	write("lhm-0.21.2/beszel_lhm.exe")
	write("lhm-0.21.2/LibreHardwareMonitorLib.dll")
	// unrelated entries
	write("smartmontools/smartctl.exe")
	write("notes.txt")

	removeOutdatedLHM(base, "lhm-0.21.2")

	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(base, filepath.FromSlash(name)))
		return err == nil
	}
	for _, name := range []string{"beszel_lhm.exe", "LibreHardwareMonitorLib.dll", "beszel_lhm.exe.config", "beszel_lhm.pdb", "lhm-0.20.0"} {
		assert.False(t, exists(name), "%s should be removed", name)
	}
	for _, name := range []string{"lhm-0.21.2/beszel_lhm.exe", "lhm-0.21.2/LibreHardwareMonitorLib.dll", "smartmontools/smartctl.exe", "notes.txt"} {
		assert.True(t, exists(name), "%s should be kept", name)
	}
}

func TestRemoveOutdatedLHMIgnoresErrors(t *testing.T) {
	// a missing base dir must not panic or fail
	removeOutdatedLHM(filepath.Join(t.TempDir(), "missing"), "lhm-0.21.2")

	// a locked file (here an open handle) is skipped without stopping the cleanup
	base := t.TempDir()
	locked := filepath.Join(base, "beszel_lhm.exe")
	require.NoError(t, os.WriteFile(locked, []byte("x"), 0o644))
	f, err := os.Open(locked)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	require.NoError(t, os.MkdirAll(filepath.Join(base, "lhm-0.20.0"), 0o755))
	removeOutdatedLHM(base, "lhm-0.21.2")
	_, err = os.Stat(filepath.Join(base, "lhm-0.20.0"))
	assert.True(t, os.IsNotExist(err))
}
