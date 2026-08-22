//go:build darwin

package agent

import (
	"os/exec"
	"strings"
)

func getNetworkInterfaceSpeeds() map[string]uint64 {
	result := make(map[string]uint64)
	interfaces, err := exec.Command("ifconfig", "-l").Output()
	if err != nil {
		return result
	}
	for _, name := range strings.Fields(string(interfaces)) {
		output, err := exec.Command("ifconfig", name).Output()
		if err != nil {
			continue
		}
		if speed := parseDarwinMediaSpeed(string(output)); speed > 0 {
			result[name] = speed
		}
	}
	return result
}

func parseDarwinMediaSpeed(value string) uint64 {
	for _, token := range strings.Fields(strings.ToLower(value)) {
		token = strings.Trim(token, "(),<>")
		for _, suffix := range []struct {
			name string
			bps  uint64
		}{
			{"10g", 10_000_000_000},
			{"5g", 5_000_000_000},
			{"2.5g", 2_500_000_000},
			{"2500", 2_500_000_000},
			{"1g", 1_000_000_000},
			{"1000", 1_000_000_000},
			{"100m", 100_000_000},
			{"100", 100_000_000},
			{"10m", 10_000_000},
			{"10", 10_000_000},
		} {
			if strings.HasPrefix(token, suffix.name) {
				return suffix.bps
			}
		}
	}
	return 0
}
