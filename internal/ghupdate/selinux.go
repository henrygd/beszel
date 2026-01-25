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
