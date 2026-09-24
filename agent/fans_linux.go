//go:build linux

package agent

// hwmonRoot is the sysfs entry point for hardware monitor chips. Each
// subdirectory (hwmon0, hwmon1, …) is one chip; fan*_input files inside it
// expose RPM readings.
const hwmonRoot = "/sys/class/hwmon"
