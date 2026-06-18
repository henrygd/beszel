//go:build !linux

package agent

import psutilNet "github.com/shirou/gopsutil/v4/net"

func correctNetworkCounterStat(v psutilNet.IOCountersStat) psutilNet.IOCountersStat {
	return v
}
