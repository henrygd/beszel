package hub

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/henrygd/beszel/internal/ghupdate"
	"github.com/spf13/cobra"
)

// Update updates beszel to the latest version
func Update(cmd *cobra.Command, _ []string) {
	dataDir := os.TempDir()

	// set dataDir to ./beszel_data if it exists
	if _, err := os.Stat("./beszel_data"); err == nil {
		dataDir = "./beszel_data"
	}

	// Check if china-mirrors flag is set
	useMirror, _ := cmd.Flags().GetBool("china-mirrors")

	updated, err := ghupdate.Update(ghupdate.Config{
		ArchiveExecutable: "beszel",
		DataDir:           dataDir,
		UseMirror:         useMirror,
	})
	if err != nil {
		log.Fatal(err)
	}
	if !updated {
		return
	}

	// make sure the file is executable
	exePath, err := os.Executable()
	if err == nil {
		if err := os.Chmod(exePath, 0755); err != nil {
			fmt.Printf("Warning: failed to set executable permissions: %v\n", err)
		}
	}

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
			ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel service...")
			restartCmd := exec.Command("systemctl", "restart", "beszel.service")
			if err := restartCmd.Run(); err != nil {
				ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: Failed to restart service: %v\n", err)
				ghupdate.ColorPrint(ghupdate.ColorYellow, "Please restart the service manually: sudo systemctl restart beszel")
			} else {
				ghupdate.ColorPrint(ghupdate.ColorGreen, "Service restarted successfully")
			}
			return
		}
	}

	// Check for OpenRC (Alpine Linux)
	if _, err := exec.LookPath("rc-service"); err == nil {
		cmd := exec.Command("rc-service", "beszel", "status")
		if err := cmd.Run(); err == nil {
			ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel service...")
			restartCmd := exec.Command("rc-service", "beszel", "restart")
			if err := restartCmd.Run(); err != nil {
				ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: Failed to restart service: %v\n", err)
				ghupdate.ColorPrint(ghupdate.ColorYellow, "Please restart the service manually: sudo rc-service beszel restart")
			} else {
				ghupdate.ColorPrint(ghupdate.ColorGreen, "Service restarted successfully")
			}
			return
		}
	}

	ghupdate.ColorPrint(ghupdate.ColorYellow, "Service restart not attempted. If running as a service, restart manually.")
}
