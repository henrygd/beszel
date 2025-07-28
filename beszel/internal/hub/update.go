package hub

import (
	"beszel"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
	"github.com/spf13/cobra"
)

// Update updates beszel to the latest version
func Update(_ *cobra.Command, _ []string) {
	var latest *selfupdate.Release
	var found bool
	var err error
	currentVersion := semver.MustParse(beszel.Version)
	fmt.Println("beszel", currentVersion)
	fmt.Println("Checking for updates...")
	updater, _ := selfupdate.NewUpdater(selfupdate.Config{
		Filters: []string{"beszel_"},
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

	// Try to restart the service if it's running
	restartService()
}

// restartService attempts to restart the beszel service
func restartService() {
	// Check if we're running as a service by looking for systemd
	if _, err := exec.LookPath("systemctl"); err == nil {
		// Check if beszel service exists and is active
		cmd := exec.Command("systemctl", "is-active", "beszel.service")
		if err := cmd.Run(); err == nil {
			fmt.Println("Restarting beszel service...")
			restartCmd := exec.Command("systemctl", "restart", "beszel.service")
			if err := restartCmd.Run(); err != nil {
				fmt.Printf("Warning: Failed to restart service: %v\n", err)
				fmt.Println("Please restart the service manually: sudo systemctl restart beszel")
			} else {
				fmt.Println("Service restarted successfully")
			}
			return
		}
	}

	// Check for OpenRC (Alpine Linux)
	if _, err := exec.LookPath("rc-service"); err == nil {
		cmd := exec.Command("rc-service", "beszel", "status")
		if err := cmd.Run(); err == nil {
			fmt.Println("Restarting beszel service...")
			restartCmd := exec.Command("rc-service", "beszel", "restart")
			if err := restartCmd.Run(); err != nil {
				fmt.Printf("Warning: Failed to restart service: %v\n", err)
				fmt.Println("Please restart the service manually: sudo rc-service beszel restart")
			} else {
				fmt.Println("Service restarted successfully")
			}
			return
		}
	}

	fmt.Println("Note: Service restart not attempted. If running as a service, restart manually.")
}
