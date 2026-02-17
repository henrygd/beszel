//go:build !darwin

package agent

// startPowermetricsCollector is a no-op on non-darwin platforms; the real implementation is in gpu_darwin.go.
func (gm *GPUManager) startPowermetricsCollector() {}

// startMacmonCollector is a no-op on non-darwin platforms; the real implementation is in gpu_darwin.go.
func (gm *GPUManager) startMacmonCollector() {}
