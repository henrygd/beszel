//go:build !linux

package agent

import psutilNet "github.com/shirou/gopsutil/v4/net"

func isNvidiaEthernet(name string) bool {
	return false
}

func correctNvethernetCounters(v *psutilNet.IOCountersStat) {}
