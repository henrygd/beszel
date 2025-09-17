package agent

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

func (a *Agent) updateNetworkStats(systemStats *system.Stats) {
	// network stats
	if len(a.netInterfaces) == 0 {
		// if no network interfaces, initialize again
		// this is a fix if agent started before network is online (#466)
		// maybe refactor this in the future to not cache interface names at all so we
		// don't miss an interface that's been added after agent started in any circumstance
		a.initializeNetIoStats()
	}

	if systemStats.NetworkInterfaces == nil {
		systemStats.NetworkInterfaces = make(map[string][4]uint64, 0)
	}

	if netIO, err := psutilNet.IOCounters(true); err == nil {
		msElapsed := uint64(time.Since(a.netIoStats.Time).Milliseconds())
		a.netIoStats.Time = time.Now()
		totalBytesSent := uint64(0)
		totalBytesRecv := uint64(0)
		netInterfaceDeltaTracker.Cycle()
		// sum all bytes sent and received
		for _, v := range netIO {
			// skip if not in valid network interfaces list
			if _, exists := a.netInterfaces[v.Name]; !exists {
				continue
			}
			totalBytesSent += v.BytesSent
			totalBytesRecv += v.BytesRecv

			// track deltas for each network interface
			netInterfaceDeltaTracker.Set(fmt.Sprintf("%sdown", v.Name), v.BytesRecv)
			netInterfaceDeltaTracker.Set(fmt.Sprintf("%sup", v.Name), v.BytesSent)
			var upDelta, downDelta uint64
			if msElapsed > 0 {
				upDelta = netInterfaceDeltaTracker.Delta(fmt.Sprintf("%sup", v.Name)) * 1000 / msElapsed
				downDelta = netInterfaceDeltaTracker.Delta(fmt.Sprintf("%sdown", v.Name)) * 1000 / msElapsed
			}
			// add interface to systemStats
			systemStats.NetworkInterfaces[v.Name] = [4]uint64{upDelta, downDelta, v.BytesSent, v.BytesRecv}
		}

		// add to systemStats
		var bytesSentPerSecond, bytesRecvPerSecond uint64
		if msElapsed > 0 {
			bytesSentPerSecond = (totalBytesSent - a.netIoStats.BytesSent) * 1000 / msElapsed
			bytesRecvPerSecond = (totalBytesRecv - a.netIoStats.BytesRecv) * 1000 / msElapsed
		}
		networkSentPs := bytesToMegabytes(float64(bytesSentPerSecond))
		networkRecvPs := bytesToMegabytes(float64(bytesRecvPerSecond))
		// add check for issue (#150) where sent is a massive number
		if networkSentPs > 10_000 || networkRecvPs > 10_000 {
			slog.Warn("Invalid net stats. Resetting.", "sent", networkSentPs, "recv", networkRecvPs)
			for _, v := range netIO {
				if _, exists := a.netInterfaces[v.Name]; !exists {
					continue
				}
				slog.Info(v.Name, "recv", v.BytesRecv, "sent", v.BytesSent)
			}
			// reset network I/O stats
			a.initializeNetIoStats()
		} else {
			systemStats.NetworkSent = networkSentPs
			systemStats.NetworkRecv = networkRecvPs
			systemStats.Bandwidth[0], systemStats.Bandwidth[1] = bytesSentPerSecond, bytesRecvPerSecond
			// update netIoStats
			a.netIoStats.BytesSent = totalBytesSent
			a.netIoStats.BytesRecv = totalBytesRecv
		}
	}
}

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
		strings.HasPrefix(v.Name, "bond"),
		v.BytesRecv == 0,
		v.BytesSent == 0:
		return true
	default:
		return false
	}
}
