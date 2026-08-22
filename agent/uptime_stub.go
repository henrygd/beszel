//go:build !linux

package agent

import "github.com/shirou/gopsutil/v4/host"

// getUptime returns the system uptime in seconds.
func getUptime() (uint64, error) {
	return host.Uptime()
}
