//go:build (!linux && !windows) || !amd64 || (linux && !glibc)

package agent

import "fmt"

type nvmlCollector struct {
	gm *GPUManager
}

func (c *nvmlCollector) init() error {
	return fmt.Errorf("nvml not supported on this platform")
}

func (c *nvmlCollector) start() {}
