package agent

import (
	"beszel"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

// Update updates beszel-agent to the latest version
func Update() {
	var latest *selfupdate.Release
	var found bool
	var err error
	currentVersion := semver.MustParse(beszel.Version)
	fmt.Println("beszel-agent", currentVersion)
	fmt.Println("Checking for updates...")
	updater, _ := selfupdate.NewUpdater(selfupdate.Config{
		Filters: []string{"beszel-agent"},
	})
	latest, found, err = updater.DetectLatest("henrygd/beszel")

	if err != nil {
		fmt.Println("Error checking for updates:", err)
		os.Exit(1)
	}

	if !found {
		fmt.Println("No updates found")
		os.Exit(0)
	}

	fmt.Println("Latest version:", latest.Version)

	if latest.Version.LTE(currentVersion) {
		fmt.Println("You are up to date")
		return
	}

	var binaryPath string
	fmt.Printf("Updating from %s to %s...\n", currentVersion, latest.Version)
	binaryPath, err = os.Executable()
	if err != nil {
		fmt.Println("Error getting binary path:", err)
		os.Exit(1)
	}
	err = selfupdate.UpdateTo(latest.AssetURL, binaryPath)
	if err != nil {
		fmt.Println("Please try rerunning with sudo. Error:", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully updated to %s\n\n%s\n", latest.Version, strings.TrimSpace(latest.ReleaseNotes))

	// Handle SELinux context if needed
	handleSELinuxContext()

	// Try to restart the service if it's running
	restartService()
}

// restartService attempts to restart the beszel-agent service
func restartService() {
	// Check if we're running as a service by looking for systemd
	if _, err := exec.LookPath("systemctl"); err == nil {
		// Check if beszel-agent service exists and is active
		cmd := exec.Command("systemctl", "is-active", "beszel-agent.service")
		if err := cmd.Run(); err == nil {
			fmt.Println("Restarting beszel-agent service...")
			restartCmd := exec.Command("systemctl", "restart", "beszel-agent.service")
			if err := restartCmd.Run(); err != nil {
				fmt.Printf("Warning: Failed to restart service: %v\n", err)
				fmt.Println("Please restart the service manually: sudo systemctl restart beszel-agent")
			} else {
				fmt.Println("Service restarted successfully")
			}
			return
		}
	}

	// Check for OpenRC (Alpine Linux)
	if _, err := exec.LookPath("rc-service"); err == nil {
		cmd := exec.Command("rc-service", "beszel-agent", "status")
		if err := cmd.Run(); err == nil {
			fmt.Println("Restarting beszel-agent service...")
			restartCmd := exec.Command("rc-service", "beszel-agent", "restart")
			if err := restartCmd.Run(); err != nil {
				fmt.Printf("Warning: Failed to restart service: %v\n", err)
				fmt.Println("Please restart the service manually: sudo rc-service beszel-agent restart")
			} else {
				fmt.Println("Service restarted successfully")
			}
			return
		}
	}

	// Check for OpenWRT procd
	if _, err := exec.LookPath("service"); err == nil {
		cmd := exec.Command("service", "beszel-agent", "running")
		if err := cmd.Run(); err == nil {
			fmt.Println("Restarting beszel-agent service...")
			restartCmd := exec.Command("service", "beszel-agent", "restart")
			if err := restartCmd.Run(); err != nil {
				fmt.Printf("Warning: Failed to restart service: %v\n", err)
				fmt.Println("Please restart the service manually: sudo service beszel-agent restart")
			} else {
				fmt.Println("Service restarted successfully")
			}
			return
		}
	}

	fmt.Println("Note: Service restart not attempted. If running as a service, restart manually.")
}

// handleSELinuxContext applies SELinux context if SELinux is enabled
func handleSELinuxContext() {
	// Check if SELinux is enabled
	cmd := exec.Command("getenforce")
	output, err := cmd.Output()
	if err != nil {
		return // SELinux not available
	}

	if strings.TrimSpace(string(output)) == "Disabled" {
		return // SELinux is disabled
	}

	fmt.Println("SELinux enabled, applying context...")

	// Try chcon first
	if chconCmd, err := exec.LookPath("chcon"); err == nil {
		binaryPath, _ := os.Executable()
		chconExec := exec.Command(chconCmd, "-t", "bin_t", binaryPath)
		if err := chconExec.Run(); err != nil {
			fmt.Println("Warning: chcon command failed to apply context.")
		}
	}

	// Try restorecon as well
	if restoreconCmd, err := exec.LookPath("restorecon"); err == nil {
		binaryPath, _ := os.Executable()
		restoreconExec := exec.Command(restoreconCmd, "-v", binaryPath)
		restoreconExec.Stdout = nil
		restoreconExec.Stderr = nil
		if err := restoreconExec.Run(); err != nil {
			fmt.Println("Warning: restorecon command failed to apply context.")
		}
	}
}
