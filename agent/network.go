package agent

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
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
		bytesSentPerSecond += upDelta
		bytesRecvPerSecond += downDelta
	}

	return bytesSentPerSecond, bytesRecvPerSecond
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

// updateTcpConnections populates systemStats.TcpConnections with TCP socket
// counts by state, and systemStats.TcpInterfaces with the state counts (no total) split by
// the network interface owning each socket's local address. Listing connections
// walks every socket (and on Linux every process fd table), so only refresh on
// the default interval. Real-time requests reuse the last snapshot.
func (a *Agent) updateTcpConnections(cacheTimeMs uint16, systemStats *system.Stats) {
	if cacheTimeMs == defaultDataCacheTimeMs {
		if conns, err := psutilNet.Connections("tcp"); err == nil {
			a.tcpConnections = countTcpConnections(conns)
			a.tcpInterfaces = countTcpConnectionsByInterface(conns, a.interfaceAddrs())
		} else {
			slog.Debug("Error getting tcp connections", "err", err)
		}
	}
	systemStats.TcpConnections = a.tcpConnections
	systemStats.TcpInterfaces = a.tcpInterfaces
}

// countTcpConnections returns [established, time_wait, total, close_wait, syn_recv, syn_sent] socket counts.
func countTcpConnections(conns []psutilNet.ConnectionStat) [6]uint32 {
	var counts [6]uint32
	for _, conn := range conns {
		// state slots map to the system tuple, skipping the total at index 2
		if slot := tcpStateSlot(conn.Status); slot >= 0 {
			counts[[5]int{0, 1, 3, 4, 5}[slot]]++
		}
	}
	counts[2] = uint32(len(conns))
	return counts
}

// tcpStateSlot returns the [established, time_wait, close_wait, syn_recv, syn_sent] slot for status, or -1 for other states.
// SYN_RECEIVED is the Windows spelling of SYN_RECV.
func tcpStateSlot(status string) int {
	switch status {
	case "ESTABLISHED":
		return 0
	case "TIME_WAIT":
		return 1
	case "CLOSE_WAIT":
		return 2
	case "SYN_RECV", "SYN_RECEIVED":
		return 3
	case "SYN_SENT":
		return 4
	}
	return -1
}

// countTcpConnectionsByInterface attributes each socket to the interface owning its
// local address, as [established, time_wait, close_wait, syn_recv, syn_sent]. Sockets in other states, or on
// wildcard or untracked addresses (e.g. 0.0.0.0 listeners, loopback, docker bridges), are skipped.
func countTcpConnectionsByInterface(conns []psutilNet.ConnectionStat, addrs map[netip.Addr]string) map[string][5]uint32 {
	if len(addrs) == 0 {
		return nil
	}
	counts := make(map[string][5]uint32)
	for _, conn := range conns {
		slot := tcpStateSlot(conn.Status)
		if slot < 0 {
			continue
		}
		ip, err := netip.ParseAddr(conn.Laddr.IP)
		if err != nil {
			continue
		}
		name, ok := addrs[ip.Unmap().WithZone("")]
		if !ok {
			continue
		}
		c := counts[name]
		c[slot]++
		counts[name] = c
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

// interfaceAddrs maps the addresses of tracked network interfaces to their names.
func (a *Agent) interfaceAddrs() map[netip.Addr]string {
	ifaces, err := net.Interfaces()
	if err != nil {
		slog.Debug("Error getting network interfaces", "err", err)
		return nil
	}
	addrs := make(map[netip.Addr]string)
	for _, iface := range ifaces {
		if _, ok := a.netInterfaces[iface.Name]; !ok {
			continue
		}
		ifaceAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range ifaceAddrs {
			if prefix, err := netip.ParsePrefix(addr.String()); err == nil {
				addrs[prefix.Addr().Unmap()] = iface.Name
			}
		}
	}
	return addrs
}
