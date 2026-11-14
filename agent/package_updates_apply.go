package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	packageUpdatesApplyTimeout = 30 * time.Minute
	defaultAutoApplyInterval   = 24 * time.Hour
	// packageApplyHelper is the only privileged entrypoint. Sudoers must allow
	// only this path with fixed argv (security|all).
	packageApplyHelper = "/usr/local/lib/beszel/beszel-pkg-apply"
)

// packageUpdatesApplyEnabled is opt-in for hub-triggered applies.
func packageUpdatesApplyEnabled() bool {
	return envTruthy("PACKAGE_UPDATES_APPLY")
}

func envTruthy(key string) bool {
	env, exists := utils.GetEnv(key)
	if !exists {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

const packageUpdatesAutoApplyFile = "package-updates-auto-apply"

// normalizeAutoApplyMode maps UI/env values to "security", "all", or "" (off).
func normalizeAutoApplyMode(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "security", "sec":
		return "security", true
	case "all", "full":
		return "all", true
	case "0", "false", "no", "off", "":
		return "", true
	default:
		return "", false
	}
}

// packageUpdatesAutoApplyModeFromEnv returns "security", "all", or "" from env only.
func packageUpdatesAutoApplyModeFromEnv() string {
	env, exists := utils.GetEnv("PACKAGE_UPDATES_AUTO_APPLY")
	if !exists {
		return ""
	}
	mode, ok := normalizeAutoApplyMode(env)
	if !ok {
		slog.Warn("Invalid PACKAGE_UPDATES_AUTO_APPLY", "value", env)
		return ""
	}
	return mode
}

// packageUpdatesAutoApplyMode is the env-only helper kept for tests/callers without a manager.
func packageUpdatesAutoApplyMode() string {
	return packageUpdatesAutoApplyModeFromEnv()
}

func (pm *packageUpdatesManager) autoApplyStatePath() string {
	if pm == nil || pm.dataDir == "" {
		return ""
	}
	return filepath.Join(pm.dataDir, packageUpdatesAutoApplyFile)
}

func (pm *packageUpdatesManager) loadAutoApplyOverrideLocked() {
	if pm.autoApplyLoaded {
		return
	}
	pm.autoApplyLoaded = true
	path := pm.autoApplyStatePath()
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	mode, ok := normalizeAutoApplyMode(string(raw))
	if !ok {
		slog.Warn("Invalid package-updates-auto-apply file", "path", path, "value", strings.TrimSpace(string(raw)))
		return
	}
	// Persist uses "off" for disabled; store sentinel so we know override is set.
	if mode == "" {
		pm.autoApplyOverride = "off"
	} else {
		pm.autoApplyOverride = mode
	}
}

// autoApplyMode returns the effective automatic apply mode for this agent.
// Prefer a UI/file override, then PACKAGE_UPDATES_AUTO_APPLY. Apply opt-in alone
// does not enable auto-apply (operators must set env or choose a mode in the UI).
func (pm *packageUpdatesManager) autoApplyMode() string {
	if pm == nil {
		return packageUpdatesAutoApplyModeFromEnv()
	}
	pm.Lock()
	defer pm.Unlock()
	return pm.autoApplyModeLocked()
}

func (pm *packageUpdatesManager) autoApplyModeLocked() string {
	pm.loadAutoApplyOverrideLocked()
	if pm.autoApplyOverride != "" {
		if pm.autoApplyOverride == "off" {
			return ""
		}
		return pm.autoApplyOverride
	}
	return packageUpdatesAutoApplyModeFromEnv()
}

// setAutoApplyMode persists and applies a hub-selected auto-apply mode ("all"|"security"|"off").
func (pm *packageUpdatesManager) setAutoApplyMode(mode string) system.PackageUpdatesAutoApplyResult {
	result := system.PackageUpdatesAutoApplyResult{}
	if pm == nil {
		result.Error = "package updates are not available on this agent"
		return result
	}
	if !packageUpdatesApplyEnabled() {
		result.Error = "package update apply is disabled (set PACKAGE_UPDATES_APPLY=true on the agent)"
		return result
	}
	normalized, ok := normalizeAutoApplyMode(mode)
	if !ok {
		result.Error = "mode must be \"all\", \"security\", or \"off\""
		return result
	}
	display := normalized
	if display == "" {
		display = "off"
	}
	path := pm.autoApplyStatePath()
	if path == "" {
		result.Error = "agent data directory is not available"
		return result
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		result.Error = err.Error()
		return result
	}
	if err := os.WriteFile(path, []byte(display+"\n"), 0o644); err != nil {
		result.Error = err.Error()
		return result
	}

	pm.Lock()
	pm.autoApplyLoaded = true
	pm.autoApplyOverride = display
	pm.Unlock()

	result.Ok = true
	result.Mode = display
	slog.Info("Package updates auto-apply mode set", "mode", display)
	return result
}

func packageUpdatesAutoApplyInterval() time.Duration {
	env, exists := utils.GetEnv("PACKAGE_UPDATES_AUTO_APPLY_INTERVAL")
	if !exists {
		return defaultAutoApplyInterval
	}
	d, err := time.ParseDuration(env)
	if err != nil || d <= 0 {
		slog.Warn("Invalid PACKAGE_UPDATES_AUTO_APPLY_INTERVAL", "value", env)
		return defaultAutoApplyInterval
	}
	return d
}

// applyStatus returns the latest async apply result without starting a new one.
func (pm *packageUpdatesManager) applyStatus() system.PackageUpdatesApplyResult {
	pm.Lock()
	defer pm.Unlock()
	if pm.applyResult == nil {
		return system.PackageUpdatesApplyResult{Manager: pm.name, Status: "idle"}
	}
	return *pm.applyResult
}

// startApply kicks off a background apply and returns immediately.
// Package names from the hub are ignored; only the agent's cached pending list
// is used (security filter applied locally).
func (pm *packageUpdatesManager) startApply(req common.ApplyPackageUpdatesRequest) system.PackageUpdatesApplyResult {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "all"
	}
	result := system.PackageUpdatesApplyResult{Manager: pm.name, Mode: mode, Status: "started"}

	if !packageUpdatesApplyEnabled() {
		result.Status = "error"
		result.Error = "package update apply is disabled (set PACKAGE_UPDATES_APPLY=true on the agent)"
		return result
	}
	if req.Auto && pm.autoApplyMode() == "" {
		result.Status = "error"
		result.Error = "automatic package apply is disabled"
		return result
	}
	if mode != "all" && mode != "security" {
		result.Status = "error"
		result.Error = "mode must be \"all\" or \"security\""
		return result
	}

	pm.Lock()
	if pm.applying {
		out := *pm.applyResult
		pm.Unlock()
		out.Status = "running"
		return out
	}
	pm.applying = true
	running := result
	running.Status = "running"
	pm.applyResult = &running
	packages := slices.Clone(pm.result.packages)
	securityKnown := pm.result.securityKnown
	pm.Unlock()

	// Never trust package names from the wire — only local cache.
	go pm.runApply(mode, securityKnown, packages, nil)
	return result
}

func (pm *packageUpdatesManager) runApply(mode string, securityKnown bool, packages []system.PackageUpdate, _ []string) {
	result := system.PackageUpdatesApplyResult{Manager: pm.name, Mode: mode, Status: "done"}
	defer func() {
		pm.Lock()
		pm.applyResult = &result
		pm.applying = false
		if result.Ok {
			pm.lastAutoApplyAt = time.Now()
		}
		pm.Unlock()
	}()

	targets, err := selectPackagesToApply(mode, securityKnown, packages, nil)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return
	}
	if mode == "security" && len(targets) == 0 {
		result.Ok = true
		result.Output = "no security updates pending"
		return
	}
	if mode == "all" && len(packages) == 0 && len(targets) == 0 {
		// Still allow dist-upgrade; helper is authoritative.
	}

	ctx, cancel := context.WithTimeout(context.Background(), packageUpdatesApplyTimeout)
	defer cancel()

	out, err := runPackageApplyHelper(ctx, mode, targets)
	result.Output = trimOutput(out)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return
	}

	pm.refresh()
	listed := pm.list()
	result.Ok = true
	result.Applied = len(targets)
	if result.Applied == 0 && mode == "all" {
		result.Applied = max(0, len(packages)-len(listed.Packages))
	}
	result.CheckedAt = listed.CheckedAt
	slog.Info("Package updates applied", "mode", mode, "applied", result.Applied)
}

// maybeAutoApply runs after a successful check when automatic apply is enabled.
func (pm *packageUpdatesManager) maybeAutoApply() {
	mode := pm.autoApplyMode()
	if mode == "" {
		return
	}
	interval := packageUpdatesAutoApplyInterval()

	pm.Lock()
	if pm.applying {
		pm.Unlock()
		return
	}
	if !pm.lastAutoApplyAt.IsZero() && time.Since(pm.lastAutoApplyAt) < interval {
		pm.Unlock()
		return
	}
	packages := slices.Clone(pm.result.packages)
	securityKnown := pm.result.securityKnown
	counts := slices.Clone(pm.result.counts)
	pm.Unlock()

	if len(packages) == 0 && (len(counts) == 0 || counts[0] == 0) {
		return
	}
	if mode == "security" {
		if !securityKnown {
			slog.Debug("Auto package apply skipped", "reason", "security updates not identified")
			return
		}
		n := 0
		for _, p := range packages {
			if p.Security {
				n++
			}
		}
		if n == 0 {
			return
		}
	}

	slog.Info("Automatic package apply starting", "mode", mode, "pending", len(packages))
	pm.startApply(common.ApplyPackageUpdatesRequest{Mode: mode, Auto: true})
}

func selectPackagesToApply(mode string, securityKnown bool, packages []system.PackageUpdate, requested []string) ([]string, error) {
	allow := map[string]struct{}{}
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name != "" {
			allow[name] = struct{}{}
		}
	}

	var targets []string
	for _, pkg := range packages {
		if len(allow) > 0 {
			if _, ok := allow[pkg.Name]; !ok {
				continue
			}
		}
		if mode == "security" {
			if !securityKnown {
				return nil, errors.New("security updates are not identified for this package manager; use mode=all")
			}
			if !pkg.Security {
				continue
			}
		}
		if !validPackageName(pkg.Name) {
			continue
		}
		targets = append(targets, pkg.Name)
	}
	return targets, nil
}

func validPackageName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for i, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '+' || r == ':' || r == '-'
		if !ok || (i == 0 && (r == '.' || r == '-' || r == ':' || r == '+' || r == '_')) {
			return false
		}
	}
	return true
}

// lookupSudo returns an absolute path to sudo. Systemd/agent PATH can omit
// directories that interactive shells include, and some hosts only ship sudo
// under /usr/sbin.
func lookupSudo() (string, error) {
	for _, candidate := range []string{"/usr/bin/sudo", "/usr/sbin/sudo", "/bin/sudo"} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("sudo"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("sudo not found (install the sudo package)")
}

// runPackageApplyHelper runs the constrained helper as root (directly or via sudo -n).
func runPackageApplyHelper(ctx context.Context, mode string, packages []string) (string, error) {
	helper := packageApplyHelper
	if _, err := os.Stat(helper); err != nil {
		return "", fmt.Errorf("package apply helper missing at %s (install deploy/beszel-pkg-apply)", helper)
	}
	if mode != "security" && mode != "all" {
		return "", fmt.Errorf("invalid mode %q", mode)
	}

	var stdin bytes.Buffer
	if mode == "security" {
		for _, name := range packages {
			if validPackageName(name) {
				stdin.WriteString(name)
				stdin.WriteByte('\n')
			}
		}
	}

	run := func(name string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdin = &stdin
		cmd.Env = append(os.Environ(), "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive")
		cmd.WaitDelay = 10 * time.Second
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	if os.Geteuid() == 0 {
		return run(helper, mode)
	}
	sudoPath, err := lookupSudo()
	if err != nil {
		return "", fmt.Errorf("%w (install sudoers for %s; agent must not run with NoNewPrivileges)", err, filepath.Base(helper))
	}
	out, err := run(sudoPath, "-n", helper, mode)
	if err != nil {
		return out, fmt.Errorf("%w (install sudoers for %s; agent must not run with NoNewPrivileges)", err, filepath.Base(helper))
	}
	return out, nil
}

func trimOutput(out string) string {
	const max = 8000
	out = strings.TrimSpace(out)
	if len(out) <= max {
		return out
	}
	return out[:max] + "\n... (truncated)"
}
