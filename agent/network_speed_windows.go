//go:build windows

package agent

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
)

type windowsNetworkAdapter struct {
	Name            string `json:"Name"`
	NetConnectionID string `json:"NetConnectionID"`
	Speed           uint64 `json:"Speed"`
}

func getNetworkInterfaceSpeeds() map[string]uint64 {
	// Win32_NetworkAdapter exposes the negotiated speed in bits per second and
	// matches the connection name returned by net.Interfaces on Windows.
	command := "Get-CimInstance Win32_NetworkAdapter | Select-Object Name,NetConnectionID,Speed | ConvertTo-Json -Compress"
	out, err := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command).Output()
	if err != nil {
		return nil
	}

	var raw json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}
	var adapters []windowsNetworkAdapter
	if len(raw) > 0 && raw[0] == '{' {
		var adapter windowsNetworkAdapter
		if err := json.Unmarshal(raw, &adapter); err != nil {
			return nil
		}
		adapters = []windowsNetworkAdapter{adapter}
	} else if err := json.Unmarshal(raw, &adapters); err != nil {
		return nil
	}

	result := make(map[string]uint64, len(adapters))
	for _, adapter := range adapters {
		if adapter.Speed > 0 {
			if adapter.NetConnectionID != "" {
				result[adapter.NetConnectionID] = adapter.Speed
			}
			if adapter.Name != "" {
				result[adapter.Name] = adapter.Speed
			}
		}
	}
	return result
}

// parseWindowsLinkSpeed is kept small and dependency-free for callers that
// receive a localized PowerShell link-speed string (for example, "1 Gbps").
func parseWindowsLinkSpeed(value string) uint64 {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(parts) != 2 {
		return 0
	}
	number, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || number <= 0 {
		return 0
	}
	multipliers := map[string]float64{
		"bps":  1,
		"kbps": 1_000,
		"mbps": 1_000_000,
		"gbps": 1_000_000_000,
		"tbps": 1_000_000_000_000,
	}
	return uint64(number * multipliers[parts[1]])
}
