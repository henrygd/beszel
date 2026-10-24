package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setModulesDirs points kernelModulesDirs at temp dirs containing the given releases.
func setModulesDirs(t *testing.T, releases ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, release := range releases {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, release), 0o755))
	}
	orig := kernelModulesDirs
	kernelModulesDirs = []string{dir, filepath.Join(t.TempDir(), "missing")}
	t.Cleanup(func() { kernelModulesDirs = orig })
}

func TestKernelModulesMissingFor(t *testing.T) {
	t.Run("running kernel installed", func(t *testing.T) {
		setModulesDirs(t, "6.17.1-arch1-1", "6.12.4-lts")
		assert.False(t, kernelModulesMissingFor("6.17.1-arch1-1"))
	})
	t.Run("running kernel upgraded", func(t *testing.T) {
		setModulesDirs(t, "6.17.2-arch1-1")
		assert.True(t, kernelModulesMissingFor("6.17.1-arch1-1"))
	})
	t.Run("no modules dirs", func(t *testing.T) {
		setModulesDirs(t)
		assert.False(t, kernelModulesMissingFor("6.17.1-arch1-1"))
	})
	t.Run("wsl kernel", func(t *testing.T) {
		setModulesDirs(t, "6.8.0-45-generic")
		assert.False(t, kernelModulesMissingFor("6.6.87.2-microsoft-standard-WSL2"))
	})
	t.Run("empty release", func(t *testing.T) {
		setModulesDirs(t, "6.8.0-45-generic")
		assert.False(t, kernelModulesMissingFor(""))
	})
}

func TestNewRebootCheckFileSignals(t *testing.T) {
	release := filepath.Join(t.TempDir(), "osrelease")
	require.NoError(t, os.WriteFile(release, []byte("6.8.0-45-generic\n"), 0o644))
	origRelease := kernelReleasePath
	kernelReleasePath = release
	t.Cleanup(func() { kernelReleasePath = origRelease })
	setModulesDirs(t, "6.8.0-45-generic")

	flag := filepath.Join(t.TempDir(), "reboot-flag")
	origApt, origZypper := aptRebootRequiredPath, zypperRebootNeededPath
	aptRebootRequiredPath, zypperRebootNeededPath = flag, flag
	t.Cleanup(func() { aptRebootRequiredPath, zypperRebootNeededPath = origApt, origZypper })

	ctx := context.Background()
	for _, manager := range []string{"apt", "zypper"} {
		check := newRebootCheck(manager, nil)
		assert.False(t, check(ctx), manager)
	}

	require.NoError(t, os.WriteFile(flag, nil, 0o644))
	for _, manager := range []string{"apt", "zypper"} {
		check := newRebootCheck(manager, nil)
		assert.True(t, check(ctx), manager)
	}
	// managers without their own signal only use the kernel check
	assert.False(t, newRebootCheck("pacman", nil)(ctx))

	setModulesDirs(t, "6.8.0-50-generic")
	assert.True(t, newRebootCheck("pacman", nil)(ctx))
}
