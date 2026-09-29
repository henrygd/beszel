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

	// Get the executable path before update
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

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
	if err := os.Chmod(exePath, 0755); err != nil {
		fmt.Printf("Warning: failed to set executable permissions: %v\n", err)
	}

	// Fix SELinux context if necessary
	if err := ghupdate.HandleSELinuxContext(exePath); err != nil {
		ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: SELinux context handling: %v", err)
	}

	// Try to restart the service if it's running
	restartService()
}

// restartService attempts to restart the beszel service
func restartService() {
	// Check if we're running as a service by looking for systemd
	if _, err := exec.LookPath("systemctl"); err == nil {
		// install-hub.sh names the unit beszel-hub.service. beszel.service is
		// kept as a fallback for hand written units.
		for _, unit := range []string{"beszel-hub.service", "beszel.service"} {
			if err := exec.Command("systemctl", "is-active", unit).Run(); err != nil {
				continue
			}
			reportRestart(exec.Command("systemctl", "restart", unit), "sudo systemctl restart "+unit)
			return
		}
	}

	// Check for OpenRC (Alpine Linux)
	if _, err := exec.LookPath("rc-service"); err == nil {
		for _, service := range []string{"beszel-hub", "beszel"} {
			if err := exec.Command("rc-service", service, "status").Run(); err != nil {
				continue
			}
			reportRestart(exec.Command("rc-service", service, "restart"), "sudo rc-service "+service+" restart")
			return
		}
	}

	ghupdate.ColorPrint(ghupdate.ColorYellow, "Service restart not attempted. If running as a service, restart manually.")
}

// reportRestart runs the restart command and prints the result.
func reportRestart(cmd *exec.Cmd, manualCommand string) {
	ghupdate.ColorPrint(ghupdate.ColorYellow, "Restarting beszel service...")
	if err := cmd.Run(); err != nil {
		ghupdate.ColorPrintf(ghupdate.ColorYellow, "Warning: Failed to restart service: %v\n", err)
		ghupdate.ColorPrint(ghupdate.ColorYellow, "Please restart the service manually: "+manualCommand)
	} else {
		ghupdate.ColorPrint(ghupdate.ColorGreen, "Service restarted successfully")
	}
}
