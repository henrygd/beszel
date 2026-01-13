//go:build glibc && linux && amd64

package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ebitengine/purego"
)

func openLibrary(name string) (uintptr, error) {
	return purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

func getNVMLPath() string {
	return "libnvidia-ml.so.1"
}

func hasSymbol(lib uintptr, symbol string) bool {
	_, err := purego.Dlsym(lib, symbol)
	return err == nil
}

func (c *nvmlCollector) isGPUActive(bdf string) bool {
	// runtime_status
	statusPath := filepath.Join("/sys/bus/pci/devices", bdf, "power/runtime_status")
	status, err := os.ReadFile(statusPath)
	if err != nil {
		slog.Debug("NVML: Can't read runtime_status", "bdf", bdf, "err", err)
		return true // Assume active if we can't read status
	}
	statusStr := strings.TrimSpace(string(status))
	if statusStr != "active" && statusStr != "resuming" {
		slog.Debug("NVML: GPU not active", "bdf", bdf, "status", statusStr)
		return false
	}

	// power_state (D0 check)
	// Find any drm card device power_state
	pstatePathPattern := filepath.Join("/sys/bus/pci/devices", bdf, "drm/card*/device/power_state")
	matches, _ := filepath.Glob(pstatePathPattern)
	if len(matches) > 0 {
		pstate, err := os.ReadFile(matches[0])
		if err == nil {
			pstateStr := strings.TrimSpace(string(pstate))
			if pstateStr != "D0" {
				slog.Debug("NVML: GPU not in D0 state", "bdf", bdf, "pstate", pstateStr)
				return false
			}
		}
	}

	return true
}
