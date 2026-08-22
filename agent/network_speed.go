package agent

import (
	"log/slog"
	"sort"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// getNetworkInterfaceDetails collects the stable identity of each interface
// separately from the high-frequency network traffic counters. Keeping this in
// system details makes the inventory useful even when an interface is idle.
func getNetworkInterfaceDetails() []system.NetworkInterface {
	nicsEnvVal, nicsEnvExists := utils.GetEnv("NICS")
	var nicCfg *NicConfig
	if nicsEnvExists {
		nicCfg = newNicConfig(nicsEnvVal)
	}

	interfaces, err := psutilNet.Interfaces()
	if err != nil {
		slog.Debug("Unable to collect network interface details", "err", err)
		return nil
	}

	speeds := getNetworkInterfaceSpeeds()
	result := make([]system.NetworkInterface, 0, len(interfaces))
	for _, iface := range interfaces {
		// Keep this inventory focused on hardware-backed interfaces. Linux
		// exposes a device link for physical NICs, so this also excludes
		// Docker veth pairs, bridges, VLANs, firewall links, and loopback
		// without depending on interface naming conventions.
		if !isPhysicalNetworkInterface(iface.Name) {
			continue
		}
		if nicCfg != nil && !isValidNic(iface.Name, nicCfg) {
			continue
		}

		addresses := make([]string, 0, len(iface.Addrs))
		for _, addr := range iface.Addrs {
			if addr.Addr != "" {
				addresses = append(addresses, addr.Addr)
			}
		}
		sort.Strings(addresses)

		result = append(result, system.NetworkInterface{
			Name:          iface.Name,
			HardwareAddr:  iface.HardwareAddr,
			MTU:           iface.MTU,
			Flags:         append([]string(nil), iface.Flags...),
			Addresses:     addresses,
			SpeedBitsPerS: speeds[iface.Name],
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
