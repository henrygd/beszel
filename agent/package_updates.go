package agent

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	defaultPackageUpdatesInterval = time.Hour
	packageUpdatesTimeout         = 5 * time.Minute
	// pacmanSyncInterval limits how often checkupdates downloads fresh sync
	// databases. Checks in between reuse the last synced copy.
	pacmanSyncInterval = 12 * time.Hour
)

// packageUpdatesResult is the outcome of one package manager check.
type packageUpdatesResult struct {
	// counts is [total] or [total, security] pending package updates.
	counts   []uint16
	packages []system.PackageUpdate
	// securityKnown is true if packages carry per-package security flags.
	securityKnown bool
}

type packageUpdatesCheck func(ctx context.Context) (packageUpdatesResult, error)

// packageUpdatesManager periodically checks the host package manager for pending
// updates in the background and caches the result, so checks never delay metrics.
type packageUpdatesManager struct {
	sync.Mutex
	name      string
	check     packageUpdatesCheck
	interval  time.Duration
	result    packageUpdatesResult
	checkedAt time.Time
	running   bool
}

// newPackageUpdatesManager returns nil if disabled or no supported package manager
// is found. Agents running in a container are skipped because the container's
// package database is not the host's. dataDir holds pacman's private sync databases.
func newPackageUpdatesManager(dataDir string) *packageUpdatesManager {
	if runtime.GOOS != "linux" || runningInContainer() {
		return nil
	}
	interval, enabled := packageUpdatesInterval()
	if !enabled {
		return nil
	}
	name, check := detectPackageManager(dataDir)
	if check == nil {
		return nil
	}
	slog.Debug("Package updates", "manager", name, "interval", interval)
	return &packageUpdatesManager{name: name, check: check, interval: interval}
}

// packageUpdatesInterval reads PACKAGE_UPDATES_INTERVAL as a Go duration such as
// "30m" or "6h". "0" disables checks. Invalid or negative values keep the default.
func packageUpdatesInterval() (interval time.Duration, enabled bool) {
	env, exists := utils.GetEnv("PACKAGE_UPDATES_INTERVAL")
	if !exists {
		return defaultPackageUpdatesInterval, true
	}
	duration, err := time.ParseDuration(env)
	switch {
	case err == nil && duration == 0:
		slog.Info("PACKAGE_UPDATES_INTERVAL", "duration", "disabled")
		return 0, false
	case err == nil && duration > 0:
		slog.Info("PACKAGE_UPDATES_INTERVAL", "duration", duration)
		return duration, true
	default:
		slog.Warn("Invalid PACKAGE_UPDATES_INTERVAL", "value", env)
		return defaultPackageUpdatesInterval, true
	}
}

// get returns the last cached counts and starts a background check if they are stale.
func (pm *packageUpdatesManager) get(now time.Time) []uint16 {
	pm.Lock()
	defer pm.Unlock()
	if !pm.running && (pm.checkedAt.IsZero() || now.Sub(pm.checkedAt) >= pm.interval) {
		pm.running = true
		go pm.refresh()
	}
	return pm.result.counts
}

// list returns the per-package details of the last check. It never starts a check.
func (pm *packageUpdatesManager) list() system.PackageUpdates {
	pm.Lock()
	defer pm.Unlock()
	data := system.PackageUpdates{
		Manager:       pm.name,
		SecurityKnown: pm.result.securityKnown,
		Packages:      pm.result.packages,
	}
	if !pm.checkedAt.IsZero() {
		data.CheckedAt = pm.checkedAt.Unix()
	}
	return data
}

func (pm *packageUpdatesManager) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), packageUpdatesTimeout)
	defer cancel()
	result, err := pm.check(ctx)
	if err != nil {
		slog.Debug("Package updates check failed", "err", err)
		result = packageUpdatesResult{}
	}
	pm.Lock()
	pm.result = result
	pm.checkedAt = time.Now()
	pm.running = false
	pm.Unlock()
}

func runningInContainer() bool {
	for _, path := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func detectPackageManager(dataDir string) (string, packageUpdatesCheck) {
	switch {
	case commandExists("apt-get"):
		return "apt", checkApt
	case commandExists("dnf"):
		return "dnf", checkDnf
	case commandExists("zypper"):
		return "zypper", checkZypper
	case commandExists("checkupdates"):
		return "pacman", newPacmanCheck(dataDir)
	case commandExists("apk"):
		return "apk", checkApk
	}
	return "", nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runPackageCommand runs a read-only package manager command and returns stdout.
// okCodes lists non-zero exit codes that still mean success.
func runPackageCommand(ctx context.Context, okCodes []int, name string, args ...string) (string, error) {
	return runPackageCommandEnv(ctx, nil, okCodes, name, args...)
}

// runPackageCommandEnv is runPackageCommand with extra environment variables.
func runPackageCommandEnv(ctx context.Context, env []string, okCodes []int, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.Env = append(cmd.Env, env...)
	// checkupdates is a shell script, so a timeout kills only the script and its
	// children can keep stdout open. WaitDelay stops Output from waiting on them.
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && slices.Contains(okCodes, exitErr.ExitCode()) {
		return string(out), nil
	}
	return string(out), err
}

// countSecurity returns the number of packages flagged as security updates.
func countSecurity(packages []system.PackageUpdate) (count uint16) {
	for _, pkg := range packages {
		if pkg.Security {
			count++
		}
	}
	return count
}

// checkApt simulates a full upgrade against the current package lists.
// It never refreshes the lists; apt-daily or the user does that.
func checkApt(ctx context.Context) (packageUpdatesResult, error) {
	out, err := runPackageCommand(ctx, nil, "apt-get", "-s", "dist-upgrade")
	if err != nil {
		return packageUpdatesResult{}, err
	}
	packages := parseAptSimulate(out)
	return packageUpdatesResult{
		counts:        []uint16{uint16(len(packages)), countSecurity(packages)},
		packages:      packages,
		securityKnown: true,
	}, nil
}

// checkDnf uses the system metadata cache only (-C), so it never downloads metadata.
// check-update lists only available versions, so installed versions come from rpm.
func checkDnf(ctx context.Context) (packageUpdatesResult, error) {
	out, err := runPackageCommand(ctx, []int{100}, "dnf", "-q", "-C", "check-update")
	if err != nil {
		return packageUpdatesResult{}, err
	}
	packages := parseDnfCheckUpdate(out)
	result := packageUpdatesResult{packages: packages}

	if len(packages) > 0 {
		args := []string{"-q", "--qf", rpmInstalledQueryFormat}
		for _, pkg := range packages {
			args = append(args, pkg.Name)
		}
		// rpm exits non-zero if any package is not installed; keep what it printed
		out, _ = runPackageCommand(ctx, nil, "rpm", args...)
		installed := parseRpmInstalled(out)
		for i := range packages {
			packages[i].Current = installed[packages[i].Name]
		}
	}

	out, err = runPackageCommand(ctx, []int{100}, "dnf", "-q", "-C", "check-update", "--security")
	if err == nil {
		// --security lists the lowest version that fixes an advisory, which may be
		// older than the version check-update offers, so match on name.arch only
		security := make(map[string]struct{})
		for _, pkg := range parseDnfCheckUpdate(out) {
			security[pkg.Name] = struct{}{}
		}
		for i := range packages {
			_, packages[i].Security = security[packages[i].Name]
		}
		result.securityKnown = true
	}
	for i := range packages {
		packages[i].Name = trimRpmArch(packages[i].Name)
	}

	result.counts = []uint16{uint16(len(packages))}
	if result.securityKnown {
		result.counts = append(result.counts, countSecurity(packages))
	}
	return result, nil
}

// checkZypper lists package updates. Security updates come from patches, which
// zypper does not map to packages here, so only the security count is known.
func checkZypper(ctx context.Context) (packageUpdatesResult, error) {
	out, err := runPackageCommand(ctx, nil, "zypper", "--no-refresh", "-q", "list-updates")
	if err != nil {
		return packageUpdatesResult{}, err
	}
	packages := parseZypperListUpdates(out)
	result := packageUpdatesResult{packages: packages, counts: []uint16{uint16(len(packages))}}
	out, err = runPackageCommand(ctx, nil, "zypper", "--no-refresh", "-q", "list-patches", "--category", "security")
	if err == nil {
		result.counts = append(result.counts, parseZypperTable(out))
	}
	return result, nil
}

// newPacmanCheck uses checkupdates (pacman-contrib), which syncs a private copy of
// the databases and never touches pacman's own. The copy lives in dataDir because
// the systemd unit's ProtectSystem=strict makes the default /tmp location read-only.
// It syncs every pacmanSyncInterval and uses the existing copy (-n) in between.
// Local upgrades show up right away since checkupdates links the live local DB.
// Exit code 2 means no updates.
func newPacmanCheck(dataDir string) packageUpdatesCheck {
	var env []string
	var syncDir string
	if dataDir != "" {
		dbPath := filepath.Join(dataDir, "checkup-db")
		env = []string{"CHECKUPDATES_DB=" + dbPath}
		syncDir = filepath.Join(dbPath, "sync")
	}
	// checks never overlap (packageUpdatesManager.running), so no lock is needed
	var lastSync time.Time
	return func(ctx context.Context) (packageUpdatesResult, error) {
		// -n with a missing database reports no updates rather than failing,
		// so always sync first and whenever the private copy is missing
		sync := lastSync.IsZero() || time.Since(lastSync) >= pacmanSyncInterval
		if !sync && syncDir != "" {
			if _, err := os.Stat(syncDir); err != nil {
				sync = true
			}
		}
		var args []string
		if !sync {
			args = append(args, "-n")
		}
		out, err := runPackageCommandEnv(ctx, env, []int{2}, "checkupdates", args...)
		if err != nil {
			return packageUpdatesResult{}, err
		}
		if sync {
			lastSync = time.Now()
		}
		packages := parsePacmanCheckUpdates(out)
		return packageUpdatesResult{counts: []uint16{uint16(len(packages))}, packages: packages}, nil
	}
}

func checkApk(ctx context.Context) (packageUpdatesResult, error) {
	out, err := runPackageCommand(ctx, nil, "apk", "--no-network", "-u", "list")
	if err != nil {
		return packageUpdatesResult{}, err
	}
	packages := parseApkUpgradable(out)
	return packageUpdatesResult{counts: []uint16{uint16(len(packages))}, packages: packages}, nil
}

// parseAptSimulate parses upgrades in `apt-get -s` output. Upgrade lines look like
// "Inst libc6 [2.35-0ubuntu3.4] (2.35-0ubuntu3.15 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [arm64])".
// New dependencies have no "[old version]" and are skipped.
func parseAptSimulate(out string) (packages []system.PackageUpdate) {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "Inst" || !strings.HasPrefix(fields[2], "[") || !strings.HasPrefix(fields[3], "(") {
			continue
		}
		pkg := system.PackageUpdate{
			Name:      fields[1],
			Current:   strings.Trim(fields[2], "[]"),
			Available: strings.TrimPrefix(fields[3], "("),
		}
		start := strings.IndexByte(line, '(')
		end := strings.IndexByte(line, ')')
		pkg.Security = start >= 0 && end > start && strings.Contains(line[start:end], "-security")
		packages = append(packages, pkg)
	}
	return packages
}

// parseDnfCheckUpdate parses "name.arch version repo" lines, stopping at the
// obsoletes section so obsoleted packages are not listed twice. Names keep the
// arch so they can be matched with rpm output. dnf4 wraps a long name.arch onto
// its own line, with the version and repo on the next line.
func parseDnfCheckUpdate(out string) (packages []system.PackageUpdate) {
	var wrappedName string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Obsoleting") {
			break
		}
		fields := strings.Fields(line)
		if wrappedName != "" && len(fields) == 2 {
			fields = []string{wrappedName, fields[0], fields[1]}
		}
		wrappedName = ""
		switch {
		case len(fields) == 3 && strings.Contains(fields[0], "."):
			packages = append(packages, system.PackageUpdate{Name: fields[0], Available: fields[1]})
		case len(fields) == 1 && strings.Contains(fields[0], ".") && !strings.HasPrefix(line, " "):
			wrappedName = fields[0]
		}
	}
	return packages
}

// rpmInstalledQueryFormat prints "name.arch [epoch:]version-release", matching
// the version format of dnf check-update.
const rpmInstalledQueryFormat = `%{NAME}.%{ARCH} %|EPOCH?{%{EPOCH}:}:{}|%{VERSION}-%{RELEASE}\n`

// parseRpmInstalled maps name.arch to its installed version. For packages with
// several installed versions, such as kernels, the last one listed wins.
func parseRpmInstalled(out string) map[string]string {
	installed := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		// "package foo.x86_64 is not installed" has more than two fields
		if fields := strings.Fields(scanner.Text()); len(fields) == 2 {
			installed[fields[0]] = fields[1]
		}
	}
	return installed
}

// trimRpmArch removes the ".arch" suffix from a dnf package name.
func trimRpmArch(name string) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[:i]
	}
	return name
}

// parseZypperTable counts the data rows of a zypper table (the lines after the
// "---+---" separator).
func parseZypperTable(out string) (count uint16) {
	inTable := false
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case !inTable:
			inTable = strings.HasPrefix(line, "--") && strings.Contains(line, "-+-")
		case strings.Contains(line, "|"):
			count++
		default:
			return count
		}
	}
	return count
}

// parseZypperListUpdates parses the `zypper list-updates` table, locating the
// columns by their header names.
func parseZypperListUpdates(out string) (packages []system.PackageUpdate) {
	nameCol, currentCol, availableCol := -1, -1, -1
	var header []string
	inTable := false
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case !inTable && strings.HasPrefix(line, "--") && strings.Contains(line, "-+-"):
			for i, col := range header {
				switch strings.TrimSpace(col) {
				case "Name":
					nameCol = i
				case "Current Version":
					currentCol = i
				case "Available Version":
					availableCol = i
				}
			}
			if nameCol < 0 || availableCol < 0 {
				return nil
			}
			inTable = true
		case !inTable:
			header = strings.Split(line, "|")
		case strings.Contains(line, "|"):
			cols := strings.Split(line, "|")
			if len(cols) != len(header) {
				continue
			}
			pkg := system.PackageUpdate{
				Name:      strings.TrimSpace(cols[nameCol]),
				Available: strings.TrimSpace(cols[availableCol]),
			}
			if currentCol >= 0 {
				pkg.Current = strings.TrimSpace(cols[currentCol])
			}
			packages = append(packages, pkg)
		default:
			return packages
		}
	}
	return packages
}

// parsePacmanCheckUpdates parses "name old -> new" lines.
func parsePacmanCheckUpdates(out string) (packages []system.PackageUpdate) {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && fields[2] == "->" {
			packages = append(packages, system.PackageUpdate{Name: fields[0], Current: fields[1], Available: fields[3]})
		}
	}
	return packages
}

// parseApkUpgradable parses lines of `apk -u list`, which look like
// "musl-1.2.5-r3 aarch64 {musl} (MIT) [upgradable from: musl-1.2.5-r0]".
func parseApkUpgradable(out string) (packages []system.PackageUpdate) {
	const marker = "[upgradable from:"
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		i := strings.Index(line, marker)
		fields := strings.Fields(line)
		if i < 0 || len(fields) == 0 {
			continue
		}
		name, available := splitApkNameVersion(fields[0])
		_, current := splitApkNameVersion(strings.TrimSuffix(strings.TrimSpace(line[i+len(marker):]), "]"))
		packages = append(packages, system.PackageUpdate{Name: name, Current: current, Available: available})
	}
	return packages
}

// splitApkNameVersion splits "name-version-rN" into name and "version-rN".
// Names may contain dashes, but versions do not.
func splitApkNameVersion(s string) (name, version string) {
	rel := strings.LastIndexByte(s, '-')
	if rel <= 0 || !strings.HasPrefix(s[rel+1:], "r") {
		return s, ""
	}
	ver := strings.LastIndexByte(s[:rel], '-')
	if ver <= 0 {
		return s, ""
	}
	return s[:ver], s[ver+1:]
}
