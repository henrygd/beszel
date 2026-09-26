//go:build testing

package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
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

// findPackage returns the named package from a parsed list.
func findPackage(t *testing.T, packages []system.PackageUpdate, name string) system.PackageUpdate {
	t.Helper()
	for _, pkg := range packages {
		if pkg.Name == name {
			return pkg
		}
	}
	t.Fatalf("package %q not found", name)
	return system.PackageUpdate{}
}

// fakeCommands puts shell scripts named after package manager commands first on PATH.
func fakeCommands(t *testing.T, scripts map[string]string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires shell scripts on PATH")
	}
	binDir := t.TempDir()
	for name, script := range scripts {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\n"+script), 0o755))
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func testDataPath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("test-data", "package_updates", name))
	require.NoError(t, err)
	return path
}

func TestPackageUpdatesInterval(t *testing.T) {
	tests := []struct {
		name     string
		value    *string
		interval time.Duration
		enabled  bool
	}{
		{"unset", nil, time.Hour, true},
		{"duration", new("30m"), 30 * time.Minute, true},
		{"compound duration", new("1h30m"), 90 * time.Minute, true},
		{"zero disables", new("0"), 0, false},
		{"zero with unit disables", new("0s"), 0, false},
		{"negative keeps default", new("-5m"), time.Hour, true},
		{"no unit keeps default", new("60"), time.Hour, true},
		{"invalid keeps default", new("hourly"), time.Hour, true},
		{"empty keeps default", new(""), time.Hour, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BESZEL_AGENT_PACKAGE_UPDATES_INTERVAL", "")
			require.NoError(t, os.Unsetenv("BESZEL_AGENT_PACKAGE_UPDATES_INTERVAL"))
			t.Setenv("PACKAGE_UPDATES_INTERVAL", "")
			require.NoError(t, os.Unsetenv("PACKAGE_UPDATES_INTERVAL"))
			if tt.value != nil {
				t.Setenv("PACKAGE_UPDATES_INTERVAL", *tt.value)
			}
			interval, enabled := packageUpdatesInterval()
			assert.Equal(t, tt.interval, interval)
			assert.Equal(t, tt.enabled, enabled)
		})
	}

	t.Run("prefixed variable takes precedence", func(t *testing.T) {
		t.Setenv("PACKAGE_UPDATES_INTERVAL", "0")
		t.Setenv("BESZEL_AGENT_PACKAGE_UPDATES_INTERVAL", "6h")
		interval, enabled := packageUpdatesInterval()
		assert.Equal(t, 6*time.Hour, interval)
		assert.True(t, enabled)
	})
}

func TestParseAptSimulate(t *testing.T) {
	tests := []struct {
		file            string
		total, security int
	}{
		{"apt_debian12.txt", 44, 5},
		{"apt_ubuntu2204.txt", 58, 45},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			packages := parseAptSimulate(readPackageUpdatesTestData(t, tt.file))
			assert.Len(t, packages, tt.total)
			assert.EqualValues(t, tt.security, countSecurity(packages))
		})
	}

	t.Run("versions", func(t *testing.T) {
		packages := parseAptSimulate(readPackageUpdatesTestData(t, "apt_ubuntu2204.txt"))
		assert.Equal(t, system.PackageUpdate{Name: "libc6", Current: "2.35-0ubuntu3.4", Available: "2.35-0ubuntu3.15", Security: true}, findPackage(t, packages, "libc6"))
		assert.Equal(t, system.PackageUpdate{Name: "base-files", Current: "12ubuntu4.4", Available: "12ubuntu4.7"}, findPackage(t, packages, "base-files"))

		packages = parseAptSimulate(readPackageUpdatesTestData(t, "apt_debian12.txt"))
		assert.Equal(t, system.PackageUpdate{Name: "tzdata", Current: "2023c-5+deb12u1", Available: "2026b-0+deb12u1"}, findPackage(t, packages, "tzdata"))
	})

	t.Run("new dependencies and trailing brackets", func(t *testing.T) {
		out := `Inst linux-image-6.8.0-50-generic (6.8.0-50.51 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst linux-image-generic [6.8.0-49.49] (6.8.0-50.50 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst gcc-12-base [12.3.0-1ubuntu1~22.04] (12.3.0-1ubuntu1~22.04.3 Ubuntu:22.04/jammy-updates [arm64]) [libstdc++6:arm64 libgcc-s1:arm64 ]
Conf linux-image-generic (6.8.0-50.50 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Remv oldpkg [1.0]`
		assert.Equal(t, []system.PackageUpdate{
			{Name: "linux-image-generic", Current: "6.8.0-49.49", Available: "6.8.0-50.50", Security: true},
			{Name: "gcc-12-base", Current: "12.3.0-1ubuntu1~22.04", Available: "12.3.0-1ubuntu1~22.04.3"},
		}, parseAptSimulate(out))
	})

	t.Run("no updates", func(t *testing.T) {
		assert.Empty(t, parseAptSimulate("Reading package lists...\n0 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.\n"))
	})
}

func TestParseDnfCheckUpdate(t *testing.T) {
	tests := []struct {
		file  string
		count int
	}{
		{"dnf4_rocky9_check_update.txt", 110},
		{"dnf4_rocky9_check_update_security.txt", 53},
		{"dnf5_fedora42_check_update.txt", 20},
		{"dnf5_fedora42_check_update_security.txt", 5},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			assert.Len(t, parseDnfCheckUpdate(readPackageUpdatesTestData(t, tt.file)), tt.count)
		})
	}

	t.Run("versions keep epoch and arch", func(t *testing.T) {
		packages := parseDnfCheckUpdate(readPackageUpdatesTestData(t, "dnf5_fedora42_check_update.txt"))
		assert.Equal(t, system.PackageUpdate{Name: "openssl-libs.aarch64", Available: "1:3.2.6-4.fc42"}, findPackage(t, packages, "openssl-libs.aarch64"))
	})

	t.Run("obsoletes section, notices and wrapped names", func(t *testing.T) {
		out := `
kernel.x86_64                     5.14.0-503.el9          baseos
Security: kernel-core-5.14.0-427.el9.x86_64 is an installed security update
python3-some-very-long-package-name-that-wraps.noarch
                                  1.2.3-4.el9             appstream
Obsoleting Packages
grub2-tools.x86_64                1:2.06-80.el9           baseos
    grub2-tools.x86_64            1:2.06-77.el9           @baseos
`
		assert.Equal(t, []system.PackageUpdate{
			{Name: "kernel.x86_64", Available: "5.14.0-503.el9"},
			{Name: "python3-some-very-long-package-name-that-wraps.noarch", Available: "1.2.3-4.el9"},
		}, parseDnfCheckUpdate(out))
	})
}

func TestParseRpmInstalled(t *testing.T) {
	installed := parseRpmInstalled(readPackageUpdatesTestData(t, "dnf4_rocky9_rpm_installed.txt"))
	assert.Len(t, installed, 110)
	assert.Equal(t, "2.34-83.el9.7", installed["glibc.aarch64"])
	assert.Equal(t, "1:3.0.7-24.el9", installed["openssl-libs.aarch64"])
	assert.NotContains(t, installed, "package")

	// several installed kernels: the last one wins
	installed = parseRpmInstalled("kernel.x86_64 5.14.0-427.el9\nkernel.x86_64 5.14.0-503.el9\n")
	assert.Equal(t, "5.14.0-503.el9", installed["kernel.x86_64"])
}

func TestCheckDnf(t *testing.T) {
	tests := []struct {
		name, updates, security, installed string
		total, securityCount               int
		pkg                                system.PackageUpdate
	}{
		{
			name:    "dnf4",
			updates: "dnf4_rocky9_check_update.txt", security: "dnf4_rocky9_check_update_security.txt", installed: "dnf4_rocky9_rpm_installed.txt",
			total: 110, securityCount: 53,
			pkg: system.PackageUpdate{Name: "vim-minimal", Current: "2:8.2.2637-20.el9_1", Available: "2:8.2.2637-26.el9_8.21", Security: true},
		},
		{
			name:    "dnf5",
			updates: "dnf5_fedora42_check_update.txt", security: "dnf5_fedora42_check_update_security.txt", installed: "dnf5_fedora42_rpm_installed.txt",
			total: 20, securityCount: 5,
			pkg: system.PackageUpdate{Name: "openssl-libs", Current: "1:3.2.6-3.fc42", Available: "1:3.2.6-4.fc42", Security: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeCommands(t, map[string]string{
				"dnf": `case "$*" in *--security*) cat "` + testDataPath(t, tt.security) + `" ;; *) cat "` + testDataPath(t, tt.updates) + `" ;; esac
exit 100`,
				"rpm": `cat "` + testDataPath(t, tt.installed) + `"
exit 1`,
			})
			result, err := checkDnf(context.Background())
			require.NoError(t, err)
			assert.Equal(t, []uint16{uint16(tt.total), uint16(tt.securityCount)}, result.counts)
			assert.True(t, result.securityKnown)
			assert.Len(t, result.packages, tt.total)
			assert.Equal(t, tt.pkg, findPackage(t, result.packages, tt.pkg.Name))
			for _, pkg := range result.packages {
				assert.NotEmpty(t, pkg.Current, pkg.Name)
			}
		})
	}

	t.Run("security query fails", func(t *testing.T) {
		fakeCommands(t, map[string]string{
			"dnf": `case "$*" in *--security*) exit 1 ;; esac
echo "bash.x86_64 5.1.8-9.el9 baseos"
exit 100`,
			"rpm": `echo "bash.x86_64 5.1.8-6.el9_1"`,
		})
		result, err := checkDnf(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []uint16{1}, result.counts)
		assert.False(t, result.securityKnown)
		assert.Equal(t, []system.PackageUpdate{{Name: "bash", Current: "5.1.8-6.el9_1", Available: "5.1.8-9.el9"}}, result.packages)
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

func TestParseZypperListUpdates(t *testing.T) {
	packages := parseZypperListUpdates(readPackageUpdatesTestData(t, "zypper_leap155_list_updates.txt"))
	assert.Len(t, packages, 22)
	assert.Equal(t, system.PackageUpdate{Name: "zypper", Current: "1.14.76-150500.6.6.15", Available: "1.14.78-150500.6.14.1"}, findPackage(t, packages, "zypper"))
	assert.Equal(t, system.PackageUpdate{Name: "aaa_base", Current: "84.87+git20180409.04c9dae-150300.10.20.1", Available: "84.87+git20180409.04c9dae-150300.10.23.1"}, findPackage(t, packages, "aaa_base"))

	assert.Empty(t, parseZypperListUpdates(readPackageUpdatesTestData(t, "zypper_leap156_list_updates_none.txt")))
	// patch tables have no version columns
	assert.Empty(t, parseZypperListUpdates(readPackageUpdatesTestData(t, "zypper_leap155_list_patches_security.txt")))
}

func TestCheckZypper(t *testing.T) {
	fakeCommands(t, map[string]string{
		"zypper": `case "$*" in *list-patches*) cat "` + testDataPath(t, "zypper_leap155_list_patches_security.txt") + `" ;; *) cat "` + testDataPath(t, "zypper_leap155_list_updates.txt") + `" ;; esac`,
	})
	result, err := checkZypper(context.Background())
	require.NoError(t, err)
	// security patches don't map to packages, so only the count is known
	assert.Equal(t, []uint16{22, 4}, result.counts)
	assert.False(t, result.securityKnown)
	assert.Len(t, result.packages, 22)
}

func TestParsePacmanCheckUpdates(t *testing.T) {
	assert.Equal(t, []system.PackageUpdate{
		{Name: "libpcap", Current: "1.10.7-1", Available: "1.11.0-1"},
		{Name: "libsecret", Current: "0.21.7-1", Available: "0.21.8.2-1"},
		{Name: "libtirpc", Current: "1.3.7-1", Available: "1.3.8-1"},
		{Name: "tzdata", Current: "2026c-1", Available: "2026d-1"},
	}, parsePacmanCheckUpdates(readPackageUpdatesTestData(t, "pacman_checkupdates.txt")))
	assert.Empty(t, parsePacmanCheckUpdates(""))
}

func TestParseApkUpgradable(t *testing.T) {
	packages := parseApkUpgradable(readPackageUpdatesTestData(t, "apk_alpine320_list_upgradable.txt"))
	assert.Len(t, packages, 10)
	assert.Equal(t, system.PackageUpdate{Name: "musl", Current: "1.2.5-r0", Available: "1.2.5-r3"}, packages[6])
	// names with dashes and digits
	assert.Equal(t, system.PackageUpdate{Name: "busybox-binsh", Current: "1.36.1-r28", Available: "1.36.1-r31"}, packages[2])
	assert.Equal(t, system.PackageUpdate{Name: "ca-certificates-bundle", Current: "20240226-r0", Available: "20260413-r0"}, packages[3])
	assert.Equal(t, system.PackageUpdate{Name: "libcrypto3", Current: "3.3.0-r2", Available: "3.3.7-r0"}, packages[4])
	assert.Empty(t, parseApkUpgradable(""))
}

func TestSplitApkNameVersion(t *testing.T) {
	tests := []struct{ in, name, version string }{
		{"musl-1.2.5-r3", "musl", "1.2.5-r3"},
		{"py3-foo-bar-2.0_rc1-r0", "py3-foo-bar", "2.0_rc1-r0"},
		{"apk-tools-2.14.4-r1", "apk-tools", "2.14.4-r1"},
		// unexpected formats keep the whole string as the name
		{"noversion", "noversion", ""},
		{"name-1.0", "name-1.0", ""},
		{"-1.0-r0", "-1.0-r0", ""},
	}
	for _, tt := range tests {
		name, version := splitApkNameVersion(tt.in)
		assert.Equal(t, tt.name, name, tt.in)
		assert.Equal(t, tt.version, version, tt.in)
	}
}

func TestPackageUpdatesManagerCaching(t *testing.T) {
	calls := make(chan struct{}, 10)
	packages := []system.PackageUpdate{{Name: "libc6", Current: "1", Available: "2", Security: true}}
	result := packageUpdatesResult{counts: []uint16{3, 1}, packages: packages, securityKnown: true}
	var resultErr error
	pm := &packageUpdatesManager{
		name:     "apt",
		interval: time.Hour,
		check: func(context.Context) (packageUpdatesResult, error) {
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

	// no check has finished yet
	assert.Equal(t, system.PackageUpdates{Manager: "apt"}, pm.list())

	now := time.Now()
	// first call starts a background check and returns nothing yet
	assert.Nil(t, pm.get(now))
	waitIdle()
	assert.Len(t, calls, 1)

	// cached result within interval, no new check
	assert.Equal(t, []uint16{3, 1}, pm.get(now.Add(time.Minute)))
	assert.Len(t, calls, 1)
	list := pm.list()
	assert.Equal(t, "apt", list.Manager)
	assert.True(t, list.SecurityKnown)
	assert.Equal(t, packages, list.Packages)
	assert.NotZero(t, list.CheckedAt)
	// list never starts a check
	assert.Len(t, calls, 1)

	// stale after interval: returns cached value and refreshes in background
	result, resultErr = packageUpdatesResult{}, errors.New("boom")
	assert.Equal(t, []uint16{3, 1}, pm.get(now.Add(2*time.Hour)))
	waitIdle()
	assert.Len(t, calls, 2)

	// failed check clears the counts and the list
	assert.Nil(t, pm.get(time.Now()))
	assert.Nil(t, pm.list().Packages)
	assert.False(t, pm.list().SecurityKnown)
}

func TestGetPackageUpdatesHandler(t *testing.T) {
	var sent any
	hctx := &HandlerContext{
		Agent: &Agent{},
		SendResponse: func(data any, _ *uint32) error {
			sent = data
			return nil
		},
	}
	handler := &GetPackageUpdatesHandler{}

	// no supported package manager
	require.NoError(t, handler.Handle(hctx))
	assert.Equal(t, system.PackageUpdates{}, sent)

	packages := []system.PackageUpdate{{Name: "musl", Current: "1.2.5-r0", Available: "1.2.5-r3"}}
	hctx.Agent.packageUpdates = &packageUpdatesManager{
		name:      "apk",
		result:    packageUpdatesResult{counts: []uint16{1}, packages: packages},
		checkedAt: time.Unix(1700000000, 0),
	}
	require.NoError(t, handler.Handle(hctx))
	assert.Equal(t, system.PackageUpdates{Manager: "apk", CheckedAt: 1700000000, Packages: packages}, sent)
}

func TestPacmanCheckSync(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a shell script on PATH")
	}
	binDir := t.TempDir()
	dataDir := t.TempDir()
	logFile := filepath.Join(binDir, "calls.log")
	// fake checkupdates logs its args and db path, and creates the sync dir when syncing
	script := `#!/bin/sh
echo "args=[$*] db=$CHECKUPDATES_DB" >> ` + logFile + `
[ "$1" = "-n" ] || mkdir -p "$CHECKUPDATES_DB/sync"
echo "linux 6.1-1 -> 6.2-1"
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "checkupdates"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	check := newPacmanCheck(dataDir)
	dbPath := filepath.Join(dataDir, "checkup-db")
	readCalls := func() []string {
		data, err := os.ReadFile(logFile)
		require.NoError(t, err)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}

	// first check syncs
	result, err := check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []uint16{1}, result.counts)
	assert.Equal(t, []system.PackageUpdate{{Name: "linux", Current: "6.1-1", Available: "6.2-1"}}, result.packages)
	// later checks reuse the synced copy
	_, err = check(context.Background())
	require.NoError(t, err)
	// a missing private copy forces a sync
	require.NoError(t, os.RemoveAll(dbPath))
	_, err = check(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{
		"args=[] db=" + dbPath,
		"args=[-n] db=" + dbPath,
		"args=[] db=" + dbPath,
	}, readCalls())
}
