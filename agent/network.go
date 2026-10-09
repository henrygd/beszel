package agent

import (
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/agent/utils"
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
		bytesSentPerSecond, bytesRecvPerSecond := a.sumAndTrackPerNicDeltas(cacheTimeMs, msElapsed, netIO, systemStats)
		a.applyNetworkTotals(cacheTimeMs, netIO, systemStats, nis, bytesSentPerSecond, bytesRecvPerSecond)
	}
}

func (a *Agent) initializeNetIoStats() {
	// reset valid network interfaces
	a.netInterfaces = make(map[string]bool, 0)

	// parse NICS env var for whitelist / blacklist
	nicsEnvVal, nicsEnvExists := utils.GetEnv("NICS")
	var nicCfg *NicConfig
	if nicsEnvExists {
		nicCfg = newNicConfig(nicsEnvVal)
	}

	// get current network I/O stats and record valid interfaces
	if netIO, err := psutilNet.IOCounters(true); err == nil {
		for _, v := range netIO {
			if skipNetworkInterface(v, nicCfg) {
				continue
			}
			// driver is checked only here so updates don't pay for it on non-Jetson systems
			useMacCounters := isNvidiaEthernet(v.Name)
			if useMacCounters {
				correctNvethernetCounters(&v)
			}
			slog.Info("Detected network interface", "name", v.Name, "sent", v.BytesSent, "recv", v.BytesRecv)
			// store as a valid network interface
			a.netInterfaces[v.Name] = useMacCounters
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
	if systemStats.NetworkInterfacePackets == nil {
		systemStats.NetworkInterfacePackets = make(map[string][6]float64, 0)
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

// sumAndTrackPerNicDeltas records per-NIC up/down deltas into systemStats and returns their summed rates.
// Summing per-NIC deltas (rather than diffing summed counters) keeps the total from underflowing
// when a tracked interface disappears or its counters reset.
func (a *Agent) sumAndTrackPerNicDeltas(cacheTimeMs uint16, msElapsed uint64, netIO []psutilNet.IOCountersStat, systemStats *system.Stats) (bytesSentPerSecond, bytesRecvPerSecond uint64) {
	tracker := a.netInterfaceDeltaTrackers[cacheTimeMs]
	if tracker == nil {
		tracker = deltatracker.NewDeltaTracker[string, uint64]()
		a.netInterfaceDeltaTrackers[cacheTimeMs] = tracker
	}
	tracker.Cycle()

	for _, v := range netIO {
		useMacCounters, exists := a.netInterfaces[v.Name]
		if !exists {
			continue
		}
		if useMacCounters {
			correctNvethernetCounters(&v)
		}
		upDelta := trackCounterDelta(tracker, v.Name+"up", v.BytesSent, msElapsed) * 1000 / max(msElapsed, 1)
		downDelta := trackCounterDelta(tracker, v.Name+"down", v.BytesRecv, msElapsed) * 1000 / max(msElapsed, 1)
		systemStats.NetworkInterfaces[v.Name] = [4]uint64{upDelta, downDelta, v.BytesSent, v.BytesRecv}
		bytesSentPerSecond += upDelta
		bytesRecvPerSecond += downDelta

		counters := [6]uint64{v.PacketsSent, v.PacketsRecv, v.Errout, v.Errin, v.Dropout, v.Dropin}
		var packetRates [6]float64
		for i, counter := range counters {
			delta := trackCounterDelta(tracker, v.Name+packetCounterKeys[i], counter, msElapsed)
			if msElapsed > 0 {
				packetRates[i] = utils.TwoDecimals(float64(delta) * 1000 / float64(msElapsed))
			}
		}
		systemStats.NetworkInterfacePackets[v.Name] = packetRates
	}

	return bytesSentPerSecond, bytesRecvPerSecond
}

// packetCounterKeys are delta tracker key suffixes, in NetworkInterfacePackets order
var packetCounterKeys = [6]string{"pktup", "pktdown", "errup", "errdown", "dropup", "dropdown"}

// trackCounterDelta records a cumulative counter value and returns its increase
// since the previous cycle. Returns 0 if there is no previous value or no time
// has elapsed. If the counter was reset, the new value is used as the delta.
func trackCounterDelta(tracker *deltatracker.DeltaTracker[string, uint64], key string, value, msElapsed uint64) uint64 {
	tracker.Set(key, value)
	if msElapsed == 0 {
		return 0
	}
	prevVal, ok := tracker.Previous(key)
	if !ok {
		return 0
	}
	if value >= prevVal {
		return value - prevVal
	}
	return value
}

// applyNetworkTotals validates and writes computed network stats, or resets on anomaly
func (a *Agent) applyNetworkTotals(
	cacheTimeMs uint16,
	netIO []psutilNet.IOCountersStat,
	systemStats *system.Stats,
	nis system.NetIoStats,
	bytesSentPerSecond, bytesRecvPerSecond uint64,
) {
	if bytesSentPerSecond > 10_000_000_000 || bytesRecvPerSecond > 10_000_000_000 {
		slog.Warn("Invalid net stats. Resetting.", "sent", bytesSentPerSecond, "recv", bytesRecvPerSecond)
		for _, v := range netIO {
			if _, exists := a.netInterfaces[v.Name]; !exists {
				continue
			}
			slog.Info(v.Name, "recv", v.BytesRecv, "sent", v.BytesSent)
		}
		a.initializeNetIoStats()
		delete(a.netIoStats, cacheTimeMs)
		delete(a.netInterfaceDeltaTrackers, cacheTimeMs)
		systemStats.Bandwidth[0], systemStats.Bandwidth[1] = 0, 0
		return
	}

	systemStats.Bandwidth[0], systemStats.Bandwidth[1] = bytesSentPerSecond, bytesRecvPerSecond
	a.netIoStats[cacheTimeMs] = nis
}

// skipNetworkInterface returns true if the network interface should be ignored.
func skipNetworkInterface(v psutilNet.IOCountersStat, nicCfg *NicConfig) bool {
	if nicCfg != nil {
		if !isValidNic(v.Name, nicCfg) {
			return true
		}
		// In whitelist mode, we honor explicit inclusion without auto-filtering.
		if !nicCfg.isBlacklist {
			return false
		}
		// In blacklist mode, still apply the auto-filter below.
	}

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
