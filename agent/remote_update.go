package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/ghupdate"
)

// Remote updates let the hub ask the agent to update itself without giving the
// agent any extra privileges. The agent only writes a trigger file; a root-owned
// systemd path unit (installed with `beszel-agent remote-update enable`) watches
// that file and runs `beszel-agent update`, which only ever installs the latest
// official release.

const (
	remoteUpdateUnitName    = "beszel-agent-remote-update"
	remoteUpdateTriggerName = "update-request"
)

var (
	// remoteUpdateUnitDir is where the systemd units are written. Variable for tests.
	remoteUpdateUnitDir = "/etc/systemd/system"
	// remoteUpdateTriggerDir must be writable by the agent service. The installer's
	// unit uses StateDirectory=beszel-agent, which makes this directory writable
	// even with ProtectSystem=strict. Variable for tests.
	remoteUpdateTriggerDir = "/var/lib/beszel-agent"
)

func remoteUpdatePathUnit() string {
	return filepath.Join(remoteUpdateUnitDir, remoteUpdateUnitName+".path")
}

func remoteUpdateServiceUnit() string {
	return filepath.Join(remoteUpdateUnitDir, remoteUpdateUnitName+".service")
}

func remoteUpdateTrigger() string {
	return filepath.Join(remoteUpdateTriggerDir, remoteUpdateTriggerName)
}

// remoteUpdateEnabled reports whether the root-owned remote update path unit is installed.
func remoteUpdateEnabled() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := os.Stat(remoteUpdatePathUnit())
	return err == nil
}

// requestRemoteUpdate writes the trigger file watched by the remote update path unit.
func requestRemoteUpdate() error {
	if !remoteUpdateEnabled() {
		return errors.New("remote update is not enabled on this agent; run `sudo beszel-agent remote-update enable` on the host")
	}
	content := time.Now().UTC().Format(time.RFC3339) + "\n"
	if err := os.WriteFile(remoteUpdateTrigger(), []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write update trigger: %w", err)
	}
	return nil
}

func remoteUpdateServiceContent(exePath string, useMirror bool) string {
	execStart := exePath + " update"
	if useMirror {
		execStart += " --china-mirrors"
	}
	return fmt.Sprintf(`[Unit]
Description=Update beszel-agent on request from the hub

[Service]
Type=oneshot
ExecStartPre=/bin/rm -f %s
ExecStart=%s
`, remoteUpdateTrigger(), execStart)
}

func remoteUpdatePathContent() string {
	return fmt.Sprintf(`[Unit]
Description=Watch for beszel-agent update requests from the hub

[Path]
PathExists=%s
Unit=%s.service

[Install]
WantedBy=multi-user.target
`, remoteUpdateTrigger(), remoteUpdateUnitName)
}

// RemoteUpdateCommand handles `beszel-agent remote-update [enable|disable|status]`.
func RemoteUpdateCommand(action string, useMirror bool) error {
	switch action {
	case "", "status":
		if remoteUpdateEnabled() {
			ghupdate.ColorPrint(ghupdate.ColorGreen, "Remote update is enabled.")
		} else {
			fmt.Println("Remote update is disabled. Enable it with: sudo beszel-agent remote-update enable")
		}
		return nil
	case "enable", "disable":
	default:
		return fmt.Errorf("unknown remote-update action %q (expected enable, disable, or status)", action)
	}

	if runtime.GOOS != "linux" {
		return errors.New("remote update is only supported on Linux with systemd")
	}
	if os.Geteuid() != 0 {
		return errors.New("remote-update " + action + " must be run as root")
	}
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return errors.New("remote update requires systemd (systemctl not found)")
	}

	if action == "enable" {
		err = enableRemoteUpdate(systemctl, useMirror)
	} else {
		err = disableRemoteUpdate(systemctl)
	}
	if err != nil {
		return err
	}

	// Restart the agent so it reports the new remote update state to the hub.
	if exec.Command(systemctl, "is-active", "--quiet", "beszel-agent.service").Run() == nil {
		if err := exec.Command(systemctl, "restart", "beszel-agent.service").Run(); err != nil {
			ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: failed to restart beszel-agent.service: %v", err)
		}
	}
	return nil
}

func enableRemoteUpdate(systemctl string, useMirror bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve agent binary path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	if strings.ContainsAny(exePath, " \t\n") {
		return fmt.Errorf("agent binary path %q must not contain whitespace", exePath)
	}

	if err := ensureTriggerDir(); err != nil {
		return err
	}
	if err := os.WriteFile(remoteUpdateServiceUnit(), []byte(remoteUpdateServiceContent(exePath, useMirror)), 0o644); err != nil {
		return fmt.Errorf("failed to write service unit: %w", err)
	}
	if err := os.WriteFile(remoteUpdatePathUnit(), []byte(remoteUpdatePathContent()), 0o644); err != nil {
		return fmt.Errorf("failed to write path unit: %w", err)
	}
	// Remove any stale trigger so enabling doesn't immediately start an update.
	_ = os.Remove(remoteUpdateTrigger())

	if out, err := exec.Command(systemctl, "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %v: %s", err, out)
	}
	if out, err := exec.Command(systemctl, "enable", "--now", remoteUpdateUnitName+".path").CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable %s.path: %v: %s", remoteUpdateUnitName, err, out)
	}
	ghupdate.ColorPrint(ghupdate.ColorGreen, "Remote update enabled. The hub can now trigger updates for this agent.")
	return nil
}

func disableRemoteUpdate(systemctl string) error {
	_ = exec.Command(systemctl, "disable", "--now", remoteUpdateUnitName+".path").Run()
	for _, path := range []string{remoteUpdatePathUnit(), remoteUpdateServiceUnit(), remoteUpdateTrigger()} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
	}
	if out, err := exec.Command(systemctl, "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %v: %s", err, out)
	}
	ghupdate.ColorPrint(ghupdate.ColorGreen, "Remote update disabled.")
	return nil
}

// ensureTriggerDir creates the trigger directory if needed and hands it to the
// beszel user so the unprivileged agent service can write the trigger file.
func ensureTriggerDir() error {
	if _, err := os.Stat(remoteUpdateTriggerDir); err == nil {
		return nil
	}
	if err := os.MkdirAll(remoteUpdateTriggerDir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", remoteUpdateTriggerDir, err)
	}
	if chown, err := exec.LookPath("chown"); err == nil {
		if err := exec.Command(chown, "beszel:beszel", remoteUpdateTriggerDir).Run(); err != nil {
			ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: failed to set ownership of %s: %v", remoteUpdateTriggerDir, err)
		}
	}
	return nil
}
