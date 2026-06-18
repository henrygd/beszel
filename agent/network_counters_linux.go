//go:build linux

package agent

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	psutilNet "github.com/shirou/gopsutil/v4/net"
)

const ethtoolTimeout = 2 * time.Second

func correctNetworkCounterStat(v psutilNet.IOCountersStat) psutilNet.IOCountersStat {
	if !isNvidiaEthernet(v.Name) {
		return v
	}

	tx, rx, ok := readEthtoolMACOctets(v.Name)
	if !ok {
		return v
	}

	v.BytesSent = tx
	v.BytesRecv = rx
	return v
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
	ctx, cancel := context.WithTimeout(context.Background(), ethtoolTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ethtool", "-S", name).Output()
	if err != nil {
		slog.Debug("Failed to read ethtool network counters", "interface", name, "err", err)
		return 0, 0, false
	}

	stats := parseEthtoolStats(string(out))
	tx, okTx := ethtoolCounter(stats, "mmc_tx_octetcount_gb", "mmc_tx_octetcount_gb_h")
	rx, okRx := ethtoolCounter(stats, "mmc_rx_octetcount_gb", "mmc_rx_octetcount_gb_h")
	if !okTx || !okRx {
		return 0, 0, false
	}
	return tx, rx, true
}

func parseEthtoolStats(out string) map[string]uint64 {
	stats := make(map[string]uint64)
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}
		parsed, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		stats[key] = parsed
	}
	return stats
}

func ethtoolCounter(stats map[string]uint64, lowKey, highKey string) (uint64, bool) {
	low, ok := stats[lowKey]
	if !ok {
		return 0, false
	}
	high, hasHigh := stats[highKey]
	if !hasHigh {
		return low, true
	}
	return high<<32 | low, true
}
