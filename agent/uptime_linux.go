//go:build linux

package agent

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/host"
)

// uptimeFilePath is a variable so tests can point it at a fixture.
var uptimeFilePath = "/proc/uptime"

// getUptime returns the system uptime in seconds.
//
// This reads /proc/uptime instead of using host.Uptime(), which calls the
// sysinfo(2) syscall. Inside an LXC container lxcfs virtualizes /proc/uptime
// but cannot intercept a syscall, so sysinfo(2) reports the host's uptime
// rather than the container's.
//
// Falls back to host.Uptime() if /proc/uptime is missing or unparseable, so
// behavior is unchanged anywhere the file isn't available.
func getUptime() (uint64, error) {
	data, err := os.ReadFile(uptimeFilePath)
	if err != nil {
		return host.Uptime()
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return host.Uptime()
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil ||
		math.IsNaN(seconds) ||
		math.IsInf(seconds, 0) ||
		seconds < 0 ||
		seconds >= 1<<64 {
		return host.Uptime()
	}
	return uint64(seconds), nil
}
