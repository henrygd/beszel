//go:build !linux

package agent

// hwmonRoot is empty on non-Linux platforms — fan RPM reporting via sysfs
// hwmon is Linux-specific. updateFans() short-circuits when this is empty.
const hwmonRoot = ""
