//go:build windows

package agent

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed smartmontools/smartctl.exe
var embeddedSmartctl []byte

var (
	smartctlOnce sync.Once
	smartctlPath string
	smartctlErr  error
)

func ensureEmbeddedSmartctl() (string, error) {
	smartctlOnce.Do(func() {
		destDir := filepath.Join(os.TempDir(), "beszel", "smartmontools")
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			smartctlErr = fmt.Errorf("failed to create smartctl directory: %w", err)
			return
		}

		destPath := filepath.Join(destDir, "smartctl.exe")
		if err := os.WriteFile(destPath, embeddedSmartctl, 0o755); err != nil {
			smartctlErr = fmt.Errorf("failed to write embedded smartctl: %w", err)
			return
		}

		smartctlPath = destPath
	})

	return smartctlPath, smartctlErr
}
