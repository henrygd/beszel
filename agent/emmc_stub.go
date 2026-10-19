//go:build !linux

package agent

import "github.com/henrygd/beszel/internal/entities/smart"

// Non-Linux builds: eMMC health via sysfs is not available.

func scanEmmcDevices() []string {
	return nil
}

func readEmmcSmartData(path string) (string, *smart.SmartData, bool) {
	return "", nil, false
}
