//go:build (!linux && !windows) || !amd64

package agent

import "fmt"

type nvmlCollector struct {
	gm *GPUManager
}

func (c *nvmlCollector) init() error {
	return fmt.Errorf("nvml not supported on this platform")
}

func (c *nvmlCollector) start() {}

func (c *nvmlCollector) collect() {}

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
