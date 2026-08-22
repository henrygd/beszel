//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func getNetworkInterfaceSpeeds() map[string]uint64 {
	return collectLinuxInterfaceSpeeds()
}

func collectLinuxInterfaceSpeeds() map[string]uint64 {
	// Linux exposes negotiated speed in /sys/class/net/<name>/speed as Mbps.
	// Virtual, loopback, and disconnected interfaces commonly report -1.
	result := make(map[string]uint64)
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return result
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join("/sys/class/net", entry.Name(), "speed"))
		if err != nil {
			continue
		}
		megabits, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil || megabits <= 0 {
			continue
		}
		result[entry.Name()] = uint64(megabits) * 1_000_000
	}
	return result
}
