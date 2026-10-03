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

type prevMemData struct {
	pswpin   uint64
	pswpout  uint64
	majFault uint64
	oomKill  uint64
	psi      [2]uint64 // cumulative stall time in microseconds [some, full]
	hasPsi   bool
	at       time.Time
}

// updateMemExtras collects swap I/O and major fault rates, OOM kill events,
// and memory pressure (PSI) as deltas over the cache interval.
// Slab memory is read directly from gopsutil in system.go.
func (a *Agent) updateMemExtras(cacheTimeMs uint16, stats *system.Stats) {
	vmstat, err := readVmstat()
	if err != nil {
		return
	}
	psi, psiErr := readMemPsiTotals()

	now := time.Now()
	prev, hasPrev := a.prevMem[cacheTimeMs]

	if hasPrev {
		elapsed := now.Sub(prev.at).Seconds()
		if elapsed > 0 {
			pageSize := float64(os.Getpagesize())
			stats.SwapIn = utils.TwoDecimals(float64(counterDelta(vmstat["pswpin"], prev.pswpin)) * pageSize / elapsed)
			stats.SwapOut = utils.TwoDecimals(float64(counterDelta(vmstat["pswpout"], prev.pswpout)) * pageSize / elapsed)
			stats.MemMajorFaults = utils.TwoDecimals(float64(counterDelta(vmstat["pgmajfault"], prev.majFault)) / elapsed)
			stats.MemOomKills = uint32(counterDelta(vmstat["oom_kill"], prev.oomKill))
			if psiErr == nil && prev.hasPsi {
				elapsedUs := elapsed * 1e6
				stats.MemPressure = []float64{
					stallPercent(counterDelta(psi[0], prev.psi[0]), elapsedUs),
					stallPercent(counterDelta(psi[1], prev.psi[1]), elapsedUs),
				}
			}
		}
	}

	a.prevMem[cacheTimeMs] = prevMemData{
		pswpin:   vmstat["pswpin"],
		pswpout:  vmstat["pswpout"],
		majFault: vmstat["pgmajfault"],
		oomKill:  vmstat["oom_kill"],
		psi:      psi,
		hasPsi:   psiErr == nil,
		at:       now,
	}
}

// counterDelta returns cur - prev, or 0 if the counter went backwards.
func counterDelta(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

// stallPercent converts a stall time delta to a percentage of the elapsed interval.
func stallPercent(stallUs uint64, elapsedUs float64) float64 {
	return utils.TwoDecimals(min(float64(stallUs)/elapsedUs*100, 100))
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
			case "pswpin", "pswpout", "pgmajfault", "oom_kill":
				if val, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
					result[parts[0]] = val
				}
			}
		}
	}
	return result, scanner.Err()
}

// readMemPsiTotals reads /proc/pressure/memory and returns the cumulative
// stall times in microseconds as [some_total, full_total].
func readMemPsiTotals() ([2]uint64, error) {
	f, err := os.Open("/proc/pressure/memory")
	if err != nil {
		return [2]uint64{}, err
	}
	defer f.Close()

	var result [2]uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) < 2 {
			continue
		}
		var total uint64
		for _, part := range parts[1:] {
			if after, ok := strings.CutPrefix(part, "total="); ok {
				total, _ = strconv.ParseUint(after, 10, 64)
			}
		}
		switch parts[0] {
		case "some":
			result[0] = total
		case "full":
			result[1] = total
		}
	}
	return result, scanner.Err()
}
