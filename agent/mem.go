package agent

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

type prevSwapData struct {
	pswpin  uint64
	pswpout uint64
	oomKill uint64
	at      time.Time
}

// updateMemExtras collects swap I/O rates, OOM kill events, and memory pressure (PSI).
// Slab memory is read directly from gopsutil in system.go.
func (a *Agent) updateMemExtras(cacheTimeMs uint16, stats *system.Stats) {
	vmstat, err := readVmstat()
	if err != nil {
		return
	}

	now := time.Now()
	prev, hasPrev := a.prevSwap[cacheTimeMs]

	if hasPrev {
		elapsed := now.Sub(prev.at).Seconds()
		if elapsed > 0 {
			pageSize := uint64(os.Getpagesize())
			swapInDelta := vmstat["pswpin"] - prev.pswpin
			swapOutDelta := vmstat["pswpout"] - prev.pswpout
			stats.SwapIn = utils.TwoDecimals(float64(swapInDelta*pageSize) / elapsed)
			stats.SwapOut = utils.TwoDecimals(float64(swapOutDelta*pageSize) / elapsed)
			stats.MemOomKills = uint32(vmstat["oom_kill"] - prev.oomKill)
		}
	}

	a.prevSwap[cacheTimeMs] = prevSwapData{
		pswpin:  vmstat["pswpin"],
		pswpout: vmstat["pswpout"],
		oomKill: vmstat["oom_kill"],
		at:      now,
	}

	if psi, err := readMemPsi(); err == nil {
		stats.MemPsi = psi[:]
	}
}

// readVmstat reads /proc/vmstat and returns selected key-value pairs.
func readVmstat() (map[string]uint64, error) {
	f, err := os.Open("/proc/vmstat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 {
			switch parts[0] {
			case "pswpin", "pswpout", "oom_kill":
				if val, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
					result[parts[0]] = val
				}
			}
		}
	}
	return result, scanner.Err()
}

// readMemPsi reads /proc/pressure/memory and returns [some_avg10, some_avg60, full_avg10, full_avg60].
func readMemPsi() ([4]float64, error) {
	f, err := os.Open("/proc/pressure/memory")
	if err != nil {
		return [4]float64{}, err
	}
	defer f.Close()

	var result [4]float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		prefix := parts[0]
		var avg10, avg60 float64
		for _, part := range parts[1:] {
			if after, ok := strings.CutPrefix(part, "avg10="); ok {
				avg10, _ = strconv.ParseFloat(after, 64)
			} else if after, ok := strings.CutPrefix(part, "avg60="); ok {
				avg60, _ = strconv.ParseFloat(after, 64)
			}
		}
		switch prefix {
		case "some":
			result[0] = avg10
			result[1] = avg60
		case "full":
			result[2] = avg10
			result[3] = avg60
		}
	}
	return result, scanner.Err()
}
