package agent

import (
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/internal/entities/system"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// NicConfig controls inclusion/exclusion of network interfaces via the NICS env var
//
// Behavior mirrors SensorConfig's matching logic:
// - Leading '-' means blacklist mode; otherwise whitelist mode
// - Supports '*' wildcards using path.Match
// - In whitelist mode with an empty list, no NICs are selected
// - In blacklist mode with an empty list, all NICs are selected
type NicConfig struct {
	nics         map[string]struct{}
	isBlacklist  bool
	hasWildcards bool
}

func newNicConfig(nicsEnvVal string) *NicConfig {
	cfg := &NicConfig{
		nics: make(map[string]struct{}),
	}
	if strings.HasPrefix(nicsEnvVal, "-") {
		cfg.isBlacklist = true
		nicsEnvVal = nicsEnvVal[1:]
	}
	for nic := range strings.SplitSeq(nicsEnvVal, ",") {
		nic = strings.TrimSpace(nic)
		if nic != "" {
			cfg.nics[nic] = struct{}{}
			if strings.Contains(nic, "*") {
				cfg.hasWildcards = true
			}
		}
	}
	return cfg
}

// isValidNic determines if a NIC should be included based on NicConfig rules
func isValidNic(nicName string, cfg *NicConfig) bool {
	// Empty list behavior differs by mode: blacklist: allow all; whitelist: allow none
	if len(cfg.nics) == 0 {
		return cfg.isBlacklist
	}

	// Exact match: return true if whitelist, false if blacklist
	if _, exactMatch := cfg.nics[nicName]; exactMatch {
		return !cfg.isBlacklist
	}

	// If no wildcards, return true if blacklist, false if whitelist
	if !cfg.hasWildcards {
		return cfg.isBlacklist
	}

	// Check for wildcard patterns
	for pattern := range cfg.nics {
		if !strings.Contains(pattern, "*") {
			continue
		}
		if match, _ := path.Match(pattern, nicName); match {
			return !cfg.isBlacklist
		}
	}

	return cfg.isBlacklist
}

func (a *Agent) updateNetworkStats(cacheTimeMs uint16, systemStats *system.Stats) {
	// network stats
	a.ensureNetInterfacesInitialized()

	a.ensureNetworkInterfacesMap(systemStats)

	if netIO, err := psutilNet.IOCounters(true); err == nil {
		nis, msElapsed := a.loadAndTickNetBaseline(cacheTimeMs)
		totalBytesSent, totalBytesRecv := a.sumAndTrackPerNicDeltas(cacheTimeMs, msElapsed, netIO, systemStats)
		bytesSentPerSecond, bytesRecvPerSecond := a.computeBytesPerSecond(msElapsed, totalBytesSent, totalBytesRecv, nis)
		a.applyNetworkTotals(cacheTimeMs, netIO, systemStats, nis, totalBytesSent, totalBytesRecv, bytesSentPerSecond, bytesRecvPerSecond)
	}
}

func (a *Agent) initializeNetIoStats() {
	// reset valid network interfaces
	a.netInterfaces = make(map[string]struct{}, 0)

	// parse NICS env var for whitelist / blacklist
	nicsEnvVal, nicsEnvExists := GetEnv("NICS")
	var nicCfg *NicConfig
	if nicsEnvExists {
		nicCfg = newNicConfig(nicsEnvVal)
	}

	// get current network I/O stats and record valid interfaces
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		for _, v := range netIO {
			if nicsEnvExists && !isValidNic(v.Name, nicCfg) {
				continue
			}
			if a.skipNetworkInterface(v) {
				continue
			}
			slog.Info("Detected network interface", "name", v.Name, "sent", v.BytesSent, "recv", v.BytesRecv)
			// store as a valid network interface
			a.netInterfaces[v.Name] = struct{}{}
		}
	}

	// Reset per-cache-time trackers and baselines so they will reinitialize on next use
	a.netInterfaceDeltaTrackers = make(map[uint16]*deltatracker.DeltaTracker[string, uint64])
	a.netIoStats = make(map[uint16]system.NetIoStats)
}

// ensureNetInterfacesInitialized re-initializes NICs if none are currently tracked
func (a *Agent) ensureNetInterfacesInitialized() {
	if len(a.netInterfaces) == 0 {
		// if no network interfaces, initialize again
		// this is a fix if agent started before network is online (#466)
		// maybe refactor this in the future to not cache interface names at all so we
		// don't miss an interface that's been added after agent started in any circumstance
		a.initializeNetIoStats()
	}
}

// ensureNetworkInterfacesMap ensures systemStats.NetworkInterfaces map exists
func (a *Agent) ensureNetworkInterfacesMap(systemStats *system.Stats) {
	if systemStats.NetworkInterfaces == nil {
		systemStats.NetworkInterfaces = make(map[string][4]uint64, 0)
	}
}

// loadAndTickNetBaseline returns the NetIoStats baseline and milliseconds elapsed, updating time
func (a *Agent) loadAndTickNetBaseline(cacheTimeMs uint16) (netIoStat system.NetIoStats, msElapsed uint64) {
	netIoStat = a.netIoStats[cacheTimeMs]
	if netIoStat.Time.IsZero() {
		netIoStat.Time = time.Now()
		msElapsed = 0
	} else {
		msElapsed = uint64(time.Since(netIoStat.Time).Milliseconds())
		netIoStat.Time = time.Now()
	}
	return netIoStat, msElapsed
}

// sumAndTrackPerNicDeltas accumulates totals and records per-NIC up/down deltas into systemStats
func (a *Agent) sumAndTrackPerNicDeltas(cacheTimeMs uint16, msElapsed uint64, netIO []psutilNet.IOCountersStat, systemStats *system.Stats) (totalBytesSent, totalBytesRecv uint64) {
	tracker := a.netInterfaceDeltaTrackers[cacheTimeMs]
	if tracker == nil {
		tracker = deltatracker.NewDeltaTracker[string, uint64]()
		a.netInterfaceDeltaTrackers[cacheTimeMs] = tracker
	}
	tracker.Cycle()

	for _, v := range netIO {
		if _, exists := a.netInterfaces[v.Name]; !exists {
			continue
		}
		totalBytesSent += v.BytesSent
		totalBytesRecv += v.BytesRecv

		var upDelta, downDelta uint64
		upKey, downKey := fmt.Sprintf("%sup", v.Name), fmt.Sprintf("%sdown", v.Name)
		tracker.Set(upKey, v.BytesSent)
		tracker.Set(downKey, v.BytesRecv)
		if msElapsed > 0 {
			if prevVal, ok := tracker.Previous(upKey); ok {
				var deltaBytes uint64
				if v.BytesSent >= prevVal {
					deltaBytes = v.BytesSent - prevVal
				} else {
					deltaBytes = v.BytesSent
				}
				upDelta = deltaBytes * 1000 / msElapsed
			}
			if prevVal, ok := tracker.Previous(downKey); ok {
				var deltaBytes uint64
				if v.BytesRecv >= prevVal {
					deltaBytes = v.BytesRecv - prevVal
				} else {
					deltaBytes = v.BytesRecv
				}
				downDelta = deltaBytes * 1000 / msElapsed
			}
		}
		systemStats.NetworkInterfaces[v.Name] = [4]uint64{upDelta, downDelta, v.BytesSent, v.BytesRecv}
	}

	return totalBytesSent, totalBytesRecv
}

// computeBytesPerSecond calculates per-second totals from elapsed time and totals
func (a *Agent) computeBytesPerSecond(msElapsed, totalBytesSent, totalBytesRecv uint64, nis system.NetIoStats) (bytesSentPerSecond, bytesRecvPerSecond uint64) {
	if msElapsed > 0 {
		bytesSentPerSecond = (totalBytesSent - nis.BytesSent) * 1000 / msElapsed
		bytesRecvPerSecond = (totalBytesRecv - nis.BytesRecv) * 1000 / msElapsed
	}
	return bytesSentPerSecond, bytesRecvPerSecond
}

// applyNetworkTotals validates and writes computed network stats, or resets on anomaly
func (a *Agent) applyNetworkTotals(
	cacheTimeMs uint16,
	netIO []psutilNet.IOCountersStat,
	systemStats *system.Stats,
	nis system.NetIoStats,
	totalBytesSent, totalBytesRecv uint64,
	bytesSentPerSecond, bytesRecvPerSecond uint64,
) {
	networkSentPs := bytesToMegabytes(float64(bytesSentPerSecond))
	networkRecvPs := bytesToMegabytes(float64(bytesRecvPerSecond))
	if networkSentPs > 10_000 || networkRecvPs > 10_000 {
		slog.Warn("Invalid net stats. Resetting.", "sent", networkSentPs, "recv", networkRecvPs)
		for _, v := range netIO {
			if _, exists := a.netInterfaces[v.Name]; !exists {
				continue
			}
			slog.Info(v.Name, "recv", v.BytesRecv, "sent", v.BytesSent)
		}
		a.initializeNetIoStats()
		delete(a.netIoStats, cacheTimeMs)
		delete(a.netInterfaceDeltaTrackers, cacheTimeMs)
		systemStats.NetworkSent = 0
		systemStats.NetworkRecv = 0
		systemStats.Bandwidth[0], systemStats.Bandwidth[1] = 0, 0
		return
	}

	systemStats.NetworkSent = networkSentPs
	systemStats.NetworkRecv = networkRecvPs
	systemStats.Bandwidth[0], systemStats.Bandwidth[1] = bytesSentPerSecond, bytesRecvPerSecond
	nis.BytesSent = totalBytesSent
	nis.BytesRecv = totalBytesRecv
	a.netIoStats[cacheTimeMs] = nis
}

func (a *Agent) skipNetworkInterface(v psutilNet.IOCountersStat) bool {
	switch {
	case strings.HasPrefix(v.Name, "lo"),
		strings.HasPrefix(v.Name, "docker"),
		strings.HasPrefix(v.Name, "br-"),
		strings.HasPrefix(v.Name, "veth"),
		strings.HasPrefix(v.Name, "bond"),
		strings.HasPrefix(v.Name, "cali"),
		v.BytesRecv == 0,
		v.BytesSent == 0:
		return true
	default:
		return false
	}
}
