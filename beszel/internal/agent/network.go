package agent

import (
	"log/slog"
	"strings"
	"time"

	psutilNet "github.com/shirou/gopsutil/v4/net"
)

func (a *Agent) initializeNetIoStats() {
	// reset valid network interfaces
	a.netInterfaces = make(map[string]struct{}, 0)

	// map of network interface names passed in via NICS env var
	var nicsMap map[string]struct{}
	nics, nicsEnvExists := GetEnv("NICS")
	if nicsEnvExists {
		nicsMap = make(map[string]struct{}, 0)
		for nic := range strings.SplitSeq(nics, ",") {
			nicsMap[nic] = struct{}{}
		}
	}

	// reset network I/O stats
	a.netIoStats.BytesSent = 0
	a.netIoStats.BytesRecv = 0

	// get intial network I/O stats
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		a.netIoStats.Time = time.Now()
		for _, v := range netIO {
			switch {
			// skip if nics exists and the interface is not in the list
			case nicsEnvExists:
				if _, nameInNics := nicsMap[v.Name]; !nameInNics {
					continue
				}
			// otherwise run the interface name through the skipNetworkInterface function
			default:
				if a.skipNetworkInterface(v) {
					continue
				}
			}
			slog.Info("Detected network interface", "name", v.Name, "sent", v.BytesSent, "recv", v.BytesRecv)
			a.netIoStats.BytesSent += v.BytesSent
			a.netIoStats.BytesRecv += v.BytesRecv
			// store as a valid network interface
			a.netInterfaces[v.Name] = struct{}{}
		}
	}
}

func (a *Agent) skipNetworkInterface(v psutilNet.IOCountersStat) bool {
	switch {
	case strings.HasPrefix(v.Name, "lo"),
		strings.HasPrefix(v.Name, "docker"),
		strings.HasPrefix(v.Name, "br-"),
		strings.HasPrefix(v.Name, "veth"),
		v.BytesRecv == 0,
		v.BytesSent == 0:
		return true
	default:
		return false
	}
}
