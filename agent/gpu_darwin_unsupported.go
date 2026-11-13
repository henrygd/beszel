//go:build !darwin

package agent

// startPowermetricsCollector is a no-op on non-darwin platforms; the real implementation is in gpu_darwin.go.
func (gm *GPUManager) startPowermetricsCollector() {}

// startMacmonCollector is a no-op on non-darwin platforms; the real implementation is in gpu_darwin.go.
func (gm *GPUManager) startMacmonCollector() {}

// pruneAppleGPUData is a no-op on non-darwin platforms.
func (gm *GPUManager) pruneAppleGPUData() {}

// isAppleGPU is false on non-darwin platforms.
func (gm *GPUManager) isAppleGPU(string) bool { return false }
