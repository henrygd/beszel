package agent

import (
	"os"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectPackagesToApply(t *testing.T) {
	packages := []system.PackageUpdate{
		{Name: "libc6", Security: true},
		{Name: "curl", Security: false},
		{Name: "openssl", Security: true},
	}

	t.Run("all", func(t *testing.T) {
		targets, err := selectPackagesToApply("all", true, packages, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"libc6", "curl", "openssl"}, targets)
	})

	t.Run("security", func(t *testing.T) {
		targets, err := selectPackagesToApply("security", true, packages, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"libc6", "openssl"}, targets)
	})

	t.Run("security unknown", func(t *testing.T) {
		_, err := selectPackagesToApply("security", false, packages, nil)
		require.Error(t, err)
	})

	t.Run("rejects unsafe names", func(t *testing.T) {
		bad := []system.PackageUpdate{{Name: "libc6;rm -rf /", Security: true}}
		targets, err := selectPackagesToApply("security", true, bad, nil)
		require.NoError(t, err)
		assert.Empty(t, targets)
	})
}

func TestValidPackageName(t *testing.T) {
	assert.True(t, validPackageName("libc6"))
	assert.True(t, validPackageName("libssl3"))
	assert.True(t, validPackageName("python3.11-minimal"))
	assert.False(t, validPackageName(""))
	assert.False(t, validPackageName("../etc/passwd"))
	assert.False(t, validPackageName("pkg;id"))
	assert.False(t, validPackageName("-bad"))
}

func TestPackageUpdatesApplyEnabled(t *testing.T) {
	t.Setenv("PACKAGE_UPDATES_APPLY", "")
	require.NoError(t, os.Unsetenv("PACKAGE_UPDATES_APPLY"))
	require.NoError(t, os.Unsetenv("BESZEL_AGENT_PACKAGE_UPDATES_APPLY"))
	assert.False(t, packageUpdatesApplyEnabled())

	t.Setenv("PACKAGE_UPDATES_APPLY", "true")
	assert.True(t, packageUpdatesApplyEnabled())
}

func TestPackageUpdatesAutoApplyMode(t *testing.T) {
	require.NoError(t, os.Unsetenv("PACKAGE_UPDATES_AUTO_APPLY"))
	require.NoError(t, os.Unsetenv("BESZEL_AGENT_PACKAGE_UPDATES_AUTO_APPLY"))
	assert.Equal(t, "", packageUpdatesAutoApplyMode())

	t.Setenv("PACKAGE_UPDATES_AUTO_APPLY", "security")
	assert.Equal(t, "security", packageUpdatesAutoApplyMode())

	t.Setenv("PACKAGE_UPDATES_AUTO_APPLY", "all")
	assert.Equal(t, "all", packageUpdatesAutoApplyMode())

	t.Setenv("PACKAGE_UPDATES_AUTO_APPLY", "false")
	assert.Equal(t, "", packageUpdatesAutoApplyMode())
}

func TestSetAutoApplyModePersists(t *testing.T) {
	dir := t.TempDir()
	pm := &packageUpdatesManager{name: "apt", dataDir: dir}

	t.Setenv("PACKAGE_UPDATES_APPLY", "true")
	result := pm.setAutoApplyMode("security")
	require.True(t, result.Ok)
	assert.Equal(t, "security", result.Mode)
	assert.Equal(t, "security", pm.autoApplyMode())

	result = pm.setAutoApplyMode("off")
	require.True(t, result.Ok)
	assert.Equal(t, "off", result.Mode)
	assert.Equal(t, "", pm.autoApplyMode())

	// Reload from disk
	pm2 := &packageUpdatesManager{name: "apt", dataDir: dir}
	assert.Equal(t, "", pm2.autoApplyMode())

	result = pm2.setAutoApplyMode("all")
	require.True(t, result.Ok)
	assert.Equal(t, "all", pm2.autoApplyMode())
}

func TestAutoApplyOffWhenOnlyApplyEnabled(t *testing.T) {
	require.NoError(t, os.Unsetenv("PACKAGE_UPDATES_AUTO_APPLY"))
	require.NoError(t, os.Unsetenv("BESZEL_AGENT_PACKAGE_UPDATES_AUTO_APPLY"))
	t.Setenv("PACKAGE_UPDATES_APPLY", "true")
	pm := &packageUpdatesManager{name: "apt", dataDir: t.TempDir()}
	assert.Equal(t, "", pm.autoApplyMode())
}
