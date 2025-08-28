package agent

import (
	"beszel/internal/ghupdate"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// restarter knows how to restart the beszel-agent service.
type restarter interface {
	Restart() error
}

type systemdRestarter struct{ cmd string }

func (s *systemdRestarter) Restart() error {
	// Only restart if the service is active
	if err := exec.Command(s.cmd, "is-active", "beszel-agent.service").Run(); err != nil {
		return nil
	}
	ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel-agent.service via systemd…")
	return exec.Command(s.cmd, "restart", "beszel-agent.service").Run()
}

type openRCRestarter struct{ cmd string }

func (o *openRCRestarter) Restart() error {
	if err := exec.Command(o.cmd, "status", "beszel-agent").Run(); err != nil {
		return nil
	}
	ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel-agent via OpenRC…")
	return exec.Command(o.cmd, "restart", "beszel-agent").Run()
}

type openWRTRestarter struct{ cmd string }

func (w *openWRTRestarter) Restart() error {
	if err := exec.Command(w.cmd, "running", "beszel-agent").Run(); err != nil {
		return nil
	}
	ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel-agent via procd…")
	return exec.Command(w.cmd, "restart", "beszel-agent").Run()
}

func detectRestarter() restarter {
	if path, err := exec.LookPath("systemctl"); err == nil {
		return &systemdRestarter{cmd: path}
	}
	if path, err := exec.LookPath("rc-service"); err == nil {
		return &openRCRestarter{cmd: path}
	}
	if path, err := exec.LookPath("service"); err == nil {
		return &openWRTRestarter{cmd: path}
	}
	return nil
}

// Update checks GitHub for a newer release of beszel-agent, applies it,
// fixes SELinux context if needed, and restarts the service.
func Update() error {
	exePath, _ := os.Executable()

	dataDir, err := getDataDir()
	if err != nil {
		dataDir = os.TempDir()
	}
	updated, err := ghupdate.Update(ghupdate.Config{
		ArchiveExecutable: "beszel-agent",
		DataDir:           dataDir,
	})
	if err != nil {
		log.Fatal(err)
	}
	if !updated {
		return nil
	}

	// make sure the file is executable
	if err := os.Chmod(exePath, 0755); err != nil {
		ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: failed to set executable permissions: %v", err)
	}
	// set ownership to beszel:beszel if possible
	if chownPath, err := exec.LookPath("chown"); err == nil {
		if err := exec.Command(chownPath, "beszel:beszel", exePath).Run(); err != nil {
			ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: failed to set file ownership: %v", err)
		}
	}

	// 6) Fix SELinux context if necessary
	if err := handleSELinuxContext(exePath); err != nil {
		ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: SELinux context handling: %v", err)
	}

	// 7) Restart service if running under a recognised init system
	if r := detectRestarter(); r != nil {
		if err := r.Restart(); err != nil {
			ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: failed to restart service: %v", err)
			ghupdate.ColorPrint(ghupdate.ColorYellow, "Please restart the service manually.")
		}
	} else {
		ghupdate.ColorPrint(ghupdate.ColorYellow, "No supported init system detected; please restart manually if needed.")
	}

	return nil
}

// handleSELinuxContext restores or applies the correct SELinux label to the binary.
func handleSELinuxContext(path string) error {
	out, err := exec.Command("getenforce").Output()
	if err != nil {
		// SELinux not enabled or getenforce not available
		return nil
	}
	state := strings.TrimSpace(string(out))
	if state == "Disabled" {
		return nil
	}

	ghupdate.ColorPrint(ghupdate.ColorYellow, "SELinux is enabled; applying context…")
	var errs []string

	// Try persistent context via semanage+restorecon
	if semanagePath, err := exec.LookPath("semanage"); err == nil {
		if err := exec.Command(semanagePath, "fcontext", "-a", "-t", "bin_t", path).Run(); err != nil {
			errs = append(errs, "semanage fcontext failed: "+err.Error())
		} else if restoreconPath, err := exec.LookPath("restorecon"); err == nil {
			if err := exec.Command(restoreconPath, "-v", path).Run(); err != nil {
				errs = append(errs, "restorecon failed: "+err.Error())
			}
		}
	}

	// Fallback to temporary context via chcon
	if chconPath, err := exec.LookPath("chcon"); err == nil {
		if err := exec.Command(chconPath, "-t", "bin_t", path).Run(); err != nil {
			errs = append(errs, "chcon failed: "+err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("SELinux context errors: %s", strings.Join(errs, "; "))
	}
	return nil
}
