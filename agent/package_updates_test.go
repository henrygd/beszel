//go:build testing

package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readPackageUpdatesTestData(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("test-data", "package_updates", name))
	require.NoError(t, err)
	return string(data)
}

// Test data files are real command outputs captured in containers.

func TestParseAptSimulate(t *testing.T) {
	tests := []struct {
		file             string
		total, security uint16
	}{
		{"apt_debian12.txt", 44, 5},
		{"apt_ubuntu2204.txt", 58, 45},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			total, security := parseAptSimulate(readPackageUpdatesTestData(t, tt.file))
			assert.Equal(t, tt.total, total)
			assert.Equal(t, tt.security, security)
		})
	}

	t.Run("new dependencies and trailing brackets", func(t *testing.T) {
		out := `Inst linux-image-6.8.0-50-generic (6.8.0-50.51 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst linux-image-generic [6.8.0-49.49] (6.8.0-50.50 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst gcc-12-base [12.3.0-1ubuntu1~22.04] (12.3.0-1ubuntu1~22.04.3 Ubuntu:22.04/jammy-updates [arm64]) [libstdc++6:arm64 libgcc-s1:arm64 ]
Conf linux-image-generic (6.8.0-50.50 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Remv oldpkg [1.0]`
		total, security := parseAptSimulate(out)
		assert.Equal(t, uint16(2), total)
		assert.Equal(t, uint16(1), security)
	})

	t.Run("no updates", func(t *testing.T) {
		total, security := parseAptSimulate("Reading package lists...\n0 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.\n")
		assert.Zero(t, total)
		assert.Zero(t, security)
	})
}

func TestParseDnfCheckUpdate(t *testing.T) {
	tests := []struct {
		file  string
		count uint16
	}{
		{"dnf4_rocky9_check_update.txt", 110},
		{"dnf4_rocky9_check_update_security.txt", 53},
		{"dnf5_fedora42_check_update.txt", 20},
		{"dnf5_fedora42_check_update_security.txt", 5},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			assert.Equal(t, tt.count, parseDnfCheckUpdate(readPackageUpdatesTestData(t, tt.file)))
		})
	}

	t.Run("obsoletes section and notices", func(t *testing.T) {
		out := `
kernel.x86_64                     5.14.0-503.el9          baseos
Security: kernel-core-5.14.0-427.el9.x86_64 is an installed security update
Obsoleting Packages
grub2-tools.x86_64                1:2.06-80.el9           baseos
    grub2-tools.x86_64            1:2.06-77.el9           @baseos
`
		assert.Equal(t, uint16(1), parseDnfCheckUpdate(out))
	})
}

func TestParseZypperTable(t *testing.T) {
	tests := []struct {
		file  string
		count uint16
	}{
		{"zypper_leap155_list_updates.txt", 22},
		{"zypper_leap155_list_patches_security.txt", 4},
		{"zypper_leap156_list_updates_none.txt", 0},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			assert.Equal(t, tt.count, parseZypperTable(readPackageUpdatesTestData(t, tt.file)))
		})
	}
}

func TestParsePacmanCheckUpdates(t *testing.T) {
	assert.Equal(t, uint16(4), parsePacmanCheckUpdates(readPackageUpdatesTestData(t, "pacman_checkupdates.txt")))
	assert.Zero(t, parsePacmanCheckUpdates(""))
}

func TestParseApkUpgradable(t *testing.T) {
	assert.Equal(t, uint16(10), parseApkUpgradable(readPackageUpdatesTestData(t, "apk_alpine320_list_upgradable.txt")))
	assert.Zero(t, parseApkUpgradable(""))
}

func TestPackageUpdatesManagerCaching(t *testing.T) {
	calls := make(chan struct{}, 10)
	result := []uint16{3, 1}
	var resultErr error
	pm := &packageUpdatesManager{
		interval: time.Hour,
		check: func(context.Context) ([]uint16, error) {
			calls <- struct{}{}
			return result, resultErr
		},
	}
	waitIdle := func() {
		require.Eventually(t, func() bool {
			pm.Lock()
			defer pm.Unlock()
			return !pm.running
		}, time.Second, time.Millisecond)
	}

	now := time.Now()
	// first call starts a background check and returns nothing yet
	assert.Nil(t, pm.get(now))
	waitIdle()
	assert.Len(t, calls, 1)

	// cached result within interval, no new check
	assert.Equal(t, []uint16{3, 1}, pm.get(now.Add(time.Minute)))
	assert.Len(t, calls, 1)

	// stale after interval: returns cached value and refreshes in background
	result, resultErr = nil, errors.New("boom")
	assert.Equal(t, []uint16{3, 1}, pm.get(now.Add(2*time.Hour)))
	waitIdle()
	assert.Len(t, calls, 2)

	// failed check clears the counts
	assert.Nil(t, pm.get(time.Now()))
}
