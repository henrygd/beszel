package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// rebootCheck reports whether the host needs a reboot to finish applying updates.
type rebootCheck func(ctx context.Context) bool

// Paths are variables so tests can point them at temp files.
var (
	// aptRebootRequiredPath is created by package hooks (update-notifier-common,
	// unattended-upgrades) on Debian-based systems.
	aptRebootRequiredPath = "/run/reboot-required"
	// zypperRebootNeededPath is created by libzypp on openSUSE when an installed
	// package has the reboot-needed flag.
	zypperRebootNeededPath = "/run/reboot-needed"
	// kernelModulesDirs hold one directory per installed kernel release.
	kernelModulesDirs = []string{"/usr/lib/modules", "/lib/modules"}
	// kernelReleasePath holds the running kernel release, as printed by uname -r.
	kernelReleasePath = "/proc/sys/kernel/osrelease"
)

// newRebootCheck returns the reboot check for a package manager. Every check falls
// back to the running kernel's modules check, which catches kernel upgrades on
// distros without their own signal.
func newRebootCheck(manager string, env []string) rebootCheck {
	switch manager {
	case "apt":
		return func(ctx context.Context) bool {
			return fileExists(aptRebootRequiredPath) || kernelModulesMissing()
		}
	case "dnf":
		return func(ctx context.Context) bool {
			return dnfNeedsRestarting(ctx, env) || kernelModulesMissing()
		}
	case "zypper":
		return func(ctx context.Context) bool {
			return fileExists(zypperRebootNeededPath) || kernelModulesMissing()
		}
	}
	return func(ctx context.Context) bool {
		return kernelModulesMissing()
	}
}

// dnfNeedsRestarting runs `dnf needs-restarting -r`, which exits 1 if core packages
// were updated since boot. dnf4 needs dnf-plugins-core for it. Any other failure,
// such as a missing plugin, is treated as no reboot needed.
func dnfNeedsRestarting(ctx context.Context, env []string) bool {
	_, err := runPackageCommandEnv(ctx, env, nil, "dnf", "-q", "-C", "needs-restarting", "-r")
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	return ok && exitErr.ExitCode() == 1
}

// kernelModulesMissing reports whether the running kernel's modules directory is gone,
// which means its package was upgraded or removed (Arch, Alpine, Void and others delete
// the old modules on upgrade). Hosts without any modules directories, such as NixOS
// or WSL, are skipped.
func kernelModulesMissing() bool {
	release, err := os.ReadFile(kernelReleasePath)
	if err != nil {
		return false
	}
	return kernelModulesMissingFor(strings.TrimSpace(string(release)))
}

func kernelModulesMissingFor(release string) bool {
	if release == "" || strings.Contains(strings.ToLower(release), "microsoft") {
		return false
	}
	hasModules := false
	for _, dir := range kernelModulesDirs {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) == 0 {
			continue
		}
		hasModules = true
		if fileExists(filepath.Join(dir, release)) {
			return false
		}
	}
	return hasModules
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
