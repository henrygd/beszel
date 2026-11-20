//go:build !linux && !darwin

package agent

// newMemBandwidthManager is supported on Linux (perf uncore PMUs, AMD APU
// gpu_metrics, Jetson actmon) and macOS (IOReport).
func newMemBandwidthManager() *memBandwidthManager {
	return nil
}
