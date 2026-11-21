//go:build !linux && !windows

package agent

// hwmonRoot is empty on non-Linux platforms — fan RPM reporting via sysfs
// hwmon is Linux-specific. updateFans() short-circuits when this is empty.
const hwmonRoot = ""

// platformFans is nil where fan speeds come from hwmon or aren't supported.
var platformFans func() map[string]uint16
