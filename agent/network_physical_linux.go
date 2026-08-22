//go:build linux

package agent

import (
	"os"
	"path/filepath"
)

const networkSysfsRoot = "/sys/class/net"

// isPhysicalNetworkInterface returns true for interfaces backed by a Linux
// kernel device. This includes normal Ethernet and wireless adapters as well
// as virtual hardware presented to a VM, while excluding software-only
// interfaces such as veth, bridges, VLANs, firewall links, and loopback.
func isPhysicalNetworkInterface(name string) bool {
	return isPhysicalNetworkInterfaceAt(networkSysfsRoot, name)
}

func isPhysicalNetworkInterfaceAt(sysfsRoot, name string) bool {
	if name == "" {
		return false
	}

	info, err := os.Stat(filepath.Join(sysfsRoot, name, "device"))
	return err == nil && info.IsDir()
}
