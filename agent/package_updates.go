package agent

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
)

const (
	defaultPackageUpdatesInterval = time.Hour
	packageUpdatesTimeout         = 5 * time.Minute
)

// packageUpdatesCheck returns [total] or [total, security] pending package updates.
type packageUpdatesCheck func(ctx context.Context) ([]uint16, error)

// packageUpdatesManager periodically checks the host package manager for pending
// updates in the background and caches the result, so checks never delay metrics.
type packageUpdatesManager struct {
	sync.Mutex
	check     packageUpdatesCheck
	interval  time.Duration
	counts    []uint16
	checkedAt time.Time
	running   bool
}

// newPackageUpdatesManager returns nil if disabled or no supported package manager
// is found. Agents running in a container are skipped because the container's
// package database is not the host's.
func newPackageUpdatesManager() *packageUpdatesManager {
	if runtime.GOOS != "linux" || runningInContainer() {
		return nil
	}
	interval := defaultPackageUpdatesInterval
	if env, exists := utils.GetEnv("PACKAGE_UPDATES_INTERVAL"); exists {
		duration, err := time.ParseDuration(env)
		switch {
		case err == nil && duration == 0:
			return nil
		case err == nil && duration > 0:
			interval = duration
		default:
			slog.Warn("Invalid PACKAGE_UPDATES_INTERVAL", "value", env)
		}
	}
	name, check := detectPackageManager()
	if check == nil {
		return nil
	}
	slog.Debug("Package updates", "manager", name, "interval", interval)
	return &packageUpdatesManager{check: check, interval: interval}
}

// get returns the last cached counts and starts a background check if they are stale.
func (pm *packageUpdatesManager) get(now time.Time) []uint16 {
	pm.Lock()
	defer pm.Unlock()
	if !pm.running && (pm.checkedAt.IsZero() || now.Sub(pm.checkedAt) >= pm.interval) {
		pm.running = true
		go pm.refresh()
	}
	return pm.counts
}

func (pm *packageUpdatesManager) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), packageUpdatesTimeout)
	defer cancel()
	counts, err := pm.check(ctx)
	if err != nil {
		slog.Debug("Package updates check failed", "err", err)
		counts = nil
	}
	pm.Lock()
	pm.counts = counts
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

func detectPackageManager() (string, packageUpdatesCheck) {
	switch {
	case commandExists("apt-get"):
		return "apt", checkApt
	case commandExists("dnf"):
		return "dnf", checkDnf
	case commandExists("zypper"):
		return "zypper", checkZypper
	case commandExists("checkupdates"):
		return "pacman", checkPacman
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
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && slices.Contains(okCodes, exitErr.ExitCode()) {
		return string(out), nil
	}
	return string(out), err
}

// checkApt simulates a full upgrade against the current package lists.
// It never refreshes the lists; apt-daily or the user does that.
func checkApt(ctx context.Context) ([]uint16, error) {
	out, err := runPackageCommand(ctx, nil, "apt-get", "-s", "dist-upgrade")
	if err != nil {
		return nil, err
	}
	total, security := parseAptSimulate(out)
	return []uint16{total, security}, nil
}

// checkDnf uses the system metadata cache only (-C), so it never downloads metadata.
func checkDnf(ctx context.Context) ([]uint16, error) {
	out, err := runPackageCommand(ctx, []int{100}, "dnf", "-q", "-C", "check-update")
	if err != nil {
		return nil, err
	}
	total := parseDnfCheckUpdate(out)
	out, err = runPackageCommand(ctx, []int{100}, "dnf", "-q", "-C", "check-update", "--security")
	if err != nil {
		return []uint16{total}, nil
	}
	return []uint16{total, parseDnfCheckUpdate(out)}, nil
}

func checkZypper(ctx context.Context) ([]uint16, error) {
	out, err := runPackageCommand(ctx, nil, "zypper", "--no-refresh", "-q", "list-updates")
	if err != nil {
		return nil, err
	}
	total := parseZypperTable(out)
	out, err = runPackageCommand(ctx, nil, "zypper", "--no-refresh", "-q", "list-patches", "--category", "security")
	if err != nil {
		return []uint16{total}, nil
	}
	return []uint16{total, parseZypperTable(out)}, nil
}

// checkPacman uses checkupdates (pacman-contrib), which syncs a private copy of
// the databases and never touches pacman's own. Exit code 2 means no updates.
func checkPacman(ctx context.Context) ([]uint16, error) {
	out, err := runPackageCommand(ctx, []int{2}, "checkupdates")
	if err != nil {
		return nil, err
	}
	return []uint16{parsePacmanCheckUpdates(out)}, nil
}

func checkApk(ctx context.Context) ([]uint16, error) {
	out, err := runPackageCommand(ctx, nil, "apk", "--no-network", "-u", "list")
	if err != nil {
		return nil, err
	}
	return []uint16{parseApkUpgradable(out)}, nil
}

// parseAptSimulate counts upgrades in `apt-get -s` output. Upgrade lines look like
// "Inst libc6 [2.35-0ubuntu3.4] (2.35-0ubuntu3.15 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [arm64])".
// New dependencies have no "[old version]" and are not counted.
func parseAptSimulate(out string) (total, security uint16) {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "Inst" || !strings.HasPrefix(fields[2], "[") {
			continue
		}
		total++
		start := strings.IndexByte(line, '(')
		end := strings.IndexByte(line, ')')
		if start >= 0 && end > start && strings.Contains(line[start:end], "-security") {
			security++
		}
	}
	return total, security
}

// parseDnfCheckUpdate counts "name.arch version repo" lines, stopping at the
// obsoletes section so obsoleted packages are not counted twice.
func parseDnfCheckUpdate(out string) (count uint16) {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Obsoleting") {
			break
		}
		fields := strings.Fields(line)
		if len(fields) == 3 && strings.Contains(fields[0], ".") {
			count++
		}
	}
	return count
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

// parsePacmanCheckUpdates counts "name old -> new" lines.
func parsePacmanCheckUpdates(out string) (count uint16) {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), " -> ") {
			count++
		}
	}
	return count
}

// parseApkUpgradable counts lines of `apk -u list`, which look like
// "musl-1.2.5-r3 aarch64 {musl} (MIT) [upgradable from: musl-1.2.5-r0]".
func parseApkUpgradable(out string) (count uint16) {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "[upgradable from:") {
			count++
		}
	}
	return count
}
