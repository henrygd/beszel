//go:build !linux && !windows

package agent

import "fmt"

func openLibrary(name string) (uintptr, error) {
	return 0, fmt.Errorf("nvml not supported on this platform")
}

func getNVMLPath() string {
	return ""
}

func hasSymbol(lib uintptr, symbol string) bool {
	return false
}

func (c *nvmlCollector) isGPUActive(bdf string) bool {
	return true
}
