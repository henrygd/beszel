//go:build linux

package agent

import (
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/safchain/ethtool"
	psutilNet "github.com/shirou/gopsutil/v4/net"
)

// correctNvethernetCounters replaces the inflated sysfs byte counters of an
// nvethernet NIC with its MAC octet counters.
func correctNvethernetCounters(v *psutilNet.IOCountersStat) {
	tx, rx, ok := readEthtoolMACOctets(v.Name)
	if !ok {
		return
	}
	v.BytesSent = tx
	v.BytesRecv = rx
}

func isNvidiaEthernet(name string) bool {
	if name == "" || strings.Contains(name, "/") {
		return false
	}

	driverPath := filepath.Join("/sys/class/net", name, "device/driver")
	target, err := os.Readlink(driverPath)
	if err != nil {
		return false
	}
	return filepath.Base(target) == "nvethernet"
}

func readEthtoolMACOctets(name string) (tx, rx uint64, ok bool) {
	stats, err := ethtool.Stats(name)
	if err != nil {
		slog.Debug("Failed to read ethtool network counters", "interface", name, "err", err)
		return 0, 0, false
	}

	tx, okTx := ethtoolCounter(stats, "mmc_tx_octetcount_gb", "mmc_tx_octetcount_gb_h")
	rx, okRx := ethtoolCounter(stats, "mmc_rx_octetcount_gb", "mmc_rx_octetcount_gb_h")
	if !okTx || !okRx {
		return 0, 0, false
	}
	return tx, rx, true
}

// ethtoolCounter combines nvethernet's split MMC counters. The driver accumulates
// the low and high registers into independent 64-bit fields, so the low value can
// exceed 32 bits and must be added rather than OR'd into the shifted high word.
func ethtoolCounter(stats map[string]uint64, lowKey, highKey string) (uint64, bool) {
	low, ok := stats[lowKey]
	if !ok {
		return 0, false
	}
	high, hasHigh := stats[highKey]
	if !hasHigh {
		return low, true
	}
	if high > (math.MaxUint64-low)>>32 {
		return 0, false
	}
	return high<<32 + low, true
}
