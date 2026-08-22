//go:build !linux

package agent

// Non-Linux platforms do not expose a portable device-backed interface path
// like /sys/class/net. Preserve the existing cross-platform behavior there;
// Linux uses the stronger hardware-backed check.
func isPhysicalNetworkInterface(_ string) bool {
	return true
}
