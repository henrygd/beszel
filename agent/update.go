package agent

import (
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/henrygd/beszel/internal/ghupdate"
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
	if err := exec.Command(o.cmd, "beszel-agent", "status").Run(); err != nil {
		return nil
	}
	ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel-agent via OpenRC…")
	return exec.Command(o.cmd, "beszel-agent", "restart").Run()
}

type openWRTRestarter struct{ cmd string }

func (w *openWRTRestarter) Restart() error {
	// https://openwrt.org/docs/guide-user/base-system/managing_services?s[]=service
	if err := exec.Command("/etc/init.d/beszel-agent", "running").Run(); err != nil {
		return nil
	}
	ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel-agent via procd…")
	return exec.Command("/etc/init.d/beszel-agent", "restart").Run()
}

type freeBSDRestarter struct{ cmd string }

func (f *freeBSDRestarter) Restart() error {
	if err := exec.Command(f.cmd, "beszel-agent", "status").Run(); err != nil {
		return nil
	}
	ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel-agent via FreeBSD rc…")
	return exec.Command(f.cmd, "beszel-agent", "restart").Run()
}

func detectRestarter() restarter {
	if path, err := exec.LookPath("systemctl"); err == nil {
		return &systemdRestarter{cmd: path}
	}
	if path, err := exec.LookPath("rc-service"); err == nil {
		return &openRCRestarter{cmd: path}
	}
    if path, err := exec.LookPath("procd"); err == nil {
        return &openWRTRestarter{cmd: path}
    }
	if path, err := exec.LookPath("service"); err == nil {
		if runtime.GOOS == "freebsd" {
			return &freeBSDRestarter{cmd: path}
		}
	}
	return nil
}

// Update checks GitHub for a newer release of beszel-agent, applies it,
// fixes SELinux context if needed, and restarts the service.
func Update(useMirror bool) error {
	exePath, _ := os.Executable()

	dataDir, err := getDataDir()
	if err != nil {
		dataDir = os.TempDir()
	}
	updated, err := ghupdate.Update(ghupdate.Config{
		ArchiveExecutable: "beszel-agent",
		DataDir:           dataDir,
		UseMirror:         useMirror,
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
	if err := ghupdate.HandleSELinuxContext(exePath); err != nil {
		ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: SELinux context handling: %v", err)
	}

	// 7) Restart service if running under a recognised init system
	if r := detectRestarter(); r != nil {
		if err := r.Restart(); err != nil {
			ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: failed to restart service: %v", err)
			ghupdate.ColorPrint(ghupdate.ColorYellow, "Please restart the service manually.")
		} else {
			ghupdate.ColorPrint(ghupdate.ColorGreen, "Service restarted successfully")
		}
	} else {
		ghupdate.ColorPrint(ghupdate.ColorYellow, "No supported init system detected; please restart manually if needed.")
	}

	return nil
}

