package agent

import psutilNet "github.com/shirou/gopsutil/v4/net"

func correctNetworkCounterStats(netIO []psutilNet.IOCountersStat) []psutilNet.IOCountersStat {
	for i := range netIO {
		netIO[i] = correctNetworkCounterStat(netIO[i])
	}
	return netIO
}
