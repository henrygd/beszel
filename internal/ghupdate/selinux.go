package ghupdate

import (
	"fmt"
	"os/exec"
	"strings"
)

// HandleSELinuxContext restores or applies the correct SELinux label to the binary.
func HandleSELinuxContext(path string) error {
	out, err := exec.Command("getenforce").Output()
	if err != nil {
		// SELinux not enabled or getenforce not available
		return nil
	}
	state := strings.TrimSpace(string(out))
	if state == "Disabled" {
		return nil
	}

	ColorPrint(ColorYellow, "SELinux is enabled; applying context…")

	// Try persistent context via semanage+restorecon
	if success := trySemanageRestorecon(path); success {
		return nil
	}

	// Fallback to temporary context via chcon
	if chconPath, err := exec.LookPath("chcon"); err == nil {
		if err := exec.Command(chconPath, "-t", "bin_t", path).Run(); err != nil {
			return fmt.Errorf("chcon failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("no SELinux tools available (semanage/restorecon or chcon)")
}

// trySemanageRestorecon attempts to set persistent SELinux context using semanage and restorecon.
// Returns true if successful, false otherwise.
func trySemanageRestorecon(path string) bool {
	semanagePath, err := exec.LookPath("semanage")
	if err != nil {
		return false
	}

	restoreconPath, err := exec.LookPath("restorecon")
	if err != nil {
		return false
	}

	// Try to add the fcontext rule; if it already exists, try to modify it
	if err := exec.Command(semanagePath, "fcontext", "-a", "-t", "bin_t", path).Run(); err != nil {
		// Rule may already exist, try modify instead
		if err := exec.Command(semanagePath, "fcontext", "-m", "-t", "bin_t", path).Run(); err != nil {
			return false
		}
	}

	// Apply the context with restorecon
	if err := exec.Command(restoreconPath, "-v", path).Run(); err != nil {
		return false
	}

	return true
}
