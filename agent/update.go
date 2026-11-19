package agent

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/henrygd/beszel/agent/utils"
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

// fetchHubVersion queries the connected hub for its version. It returns a zero
// version and an error if the hub is unreachable or does not expose its version
// (e.g. an older hub without the /api/beszel/version endpoint).
func fetchHubVersion() (semver.Version, error) {
	hubURL, exists := utils.GetEnv("HUB_URL")
	if !exists {
		return semver.Version{}, errors.New("HUB_URL not set")
	}
	u, err := url.Parse(hubURL)
	if err != nil || u.Host == "" {
		return semver.Version{}, errors.New("invalid HUB_URL")
	}
	u.Path = path.Join(u.Path, "api/beszel/version")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return semver.Version{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return semver.Version{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return semver.Version{}, errors.New("hub returned " + resp.Status)
	}

	var data struct {
		Version string `json:"v"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return semver.Version{}, err
	}
	return semver.Parse(strings.TrimPrefix(data.Version, "v"))
}

// Update checks GitHub for a newer release of beszel-agent, applies it,
// fixes SELinux context if needed, and restarts the service. When the agent
// knows its hub's version, the update is capped at the hub's release so the
// agent never runs ahead of the hub.
func Update(useMirror bool) error {
	exePath, _ := os.Executable()

	dataDir, err := GetDataDir()
	if err != nil {
		dataDir = os.TempDir()
	}

	var maxVersion string
	if hubVersion, err := fetchHubVersion(); err == nil && hubVersion.GT(semver.Version{}) {
		maxVersion = hubVersion.String()
		ghupdate.ColorPrintf(ghupdate.ColorYellow, "Capping update at hub version %s.", maxVersion)
	}

	updated, err := ghupdate.Update(ghupdate.Config{
		ArchiveExecutable: "beszel-agent",
		DataDir:           dataDir,
		UseMirror:         useMirror,
		MaxVersion:        maxVersion,
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

	// Fix SELinux context if necessary
	if err := ghupdate.HandleSELinuxContext(exePath); err != nil {
		ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: SELinux context handling: %v", err)
	}

	// Restart service if running under a recognised init system
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
