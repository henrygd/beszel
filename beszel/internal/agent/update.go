package agent

import (
    "fmt"
    "log"
    "os"
    "os/exec"
    "strings"

    "beszel"

    "github.com/blang/semver"
    "github.com/rhysd/go-github-selfupdate/selfupdate"
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
    log.Print("Restarting beszel-agent.service via systemd…")
    return exec.Command(s.cmd, "restart", "beszel-agent.service").Run()
}

type openRCRestarter struct{ cmd string }

func (o *openRCRestarter) Restart() error {
    if err := exec.Command(o.cmd, "status", "beszel-agent").Run(); err != nil {
        return nil
    }
    log.Print("Restarting beszel-agent via OpenRC…")
    return exec.Command(o.cmd, "restart", "beszel-agent").Run()
}

type openWRTRestarter struct{ cmd string }

func (w *openWRTRestarter) Restart() error {
    if err := exec.Command(w.cmd, "running", "beszel-agent").Run(); err != nil {
        return nil
    }
    log.Print("Restarting beszel-agent via procd…")
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
    // 1) Parse current version
    current, err := semver.Parse(beszel.Version)
    if err != nil {
        return fmt.Errorf("invalid current version %q: %w", beszel.Version, err)
    }
    log.Printf("Current version: %s", current)

    // 2) Create updater with our binary name filter
    updater, err := selfupdate.NewUpdater(selfupdate.Config{
        Filters: []string{"beszel-agent"},
    })
    if err != nil {
        return fmt.Errorf("creating self-update client: %w", err)
    }

    // 3) Detect latest
    log.Print("Checking for updates…")
    latest, found, err := updater.DetectLatest("henrygd/beszel")
    if err != nil {
        return fmt.Errorf("failed to detect latest release: %w", err)
    }
    if !found {
        log.Print("No updates available.")
        return nil
    }
    log.Printf("Latest version: %s", latest.Version)

    // 4) Compare versions
    if !latest.Version.GT(current) {
        log.Print("You are already up to date.")
        return nil
    }

    // 5) Perform the update
    exePath, err := os.Executable()
    if err != nil {
        return fmt.Errorf("unable to locate executable: %w", err)
    }
    log.Printf("Updating from %s to %s…", current, latest.Version)
    if err := updater.UpdateTo(latest.AssetURL, exePath); err != nil {
        return fmt.Errorf("update failed: %w", err)
    }
    log.Printf("Successfully updated to %s", latest.Version)
    log.Print("Release notes:\n", strings.TrimSpace(latest.ReleaseNotes))

    // 6) Fix SELinux context if necessary
    if err := handleSELinuxContext(exePath); err != nil {
        log.Printf("Warning: SELinux context handling: %v", err)
    }

    // 7) Restart service if running under a recognised init system
    if r := detectRestarter(); r != nil {
        if err := r.Restart(); err != nil {
            log.Printf("Warning: failed to restart service: %v", err)
            log.Print("Please restart the service manually.")
        }
    } else {
        log.Print("No supported init system detected; please restart manually if needed.")
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

    log.Print("SELinux is enabled; applying context…")
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