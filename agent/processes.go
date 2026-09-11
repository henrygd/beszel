package agent

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

const processCountsCacheDuration = 5 * time.Second

// maxProcessesForStateCounts bounds how many processes we'll scan for state counts.
// Getting a process's state requires reading /proc (or equivalent) per PID, so on
// systems with very large process counts this scan gets expensive. Above this
// limit we skip it entirely rather than pay that cost every cache interval.
const maxProcessesForStateCounts = 2000

// processCountsCache is shared across stats intervals and protected by Agent.Mutex.
// Cache failures too, so frequent live requests cannot repeatedly trigger a failed scan.
type processCountsCache struct {
	updated time.Time
	counts  [5]uint32
	err     error
}

func (c *processCountsCache) get(now time.Time, collect func() ([5]uint32, error)) ([5]uint32, error) {
	if c.updated.IsZero() || now.Sub(c.updated) >= processCountsCacheDuration {
		c.counts, c.err = collect()
		c.updated = now
	}
	return c.counts, c.err
}

// getProcessCounts returns process state counts as [running, sleeping, idle, stopped, zombie].
func getProcessCounts() ([5]uint32, error) {
	var counts [5]uint32
	pids, err := process.Pids()
	if err != nil {
		return counts, err
	}
	if len(pids) > maxProcessesForStateCounts {
		return counts, fmt.Errorf("process count %d exceeds limit %d, skipping process state scan", len(pids), maxProcessesForStateCounts)
	}
	// Construct by PID to avoid NewProcess reading creation times we never use.
	for _, pid := range pids {
		p := process.Process{Pid: pid}
		statuses, err := p.Status()
		if err != nil || len(statuses) == 0 {
			continue
		}
		switch statuses[0] {
		case process.Running:
			counts[0]++
		case process.Sleep:
			counts[1]++
		case process.Idle:
			counts[2]++
		case process.Stop:
			counts[3]++
		case process.Zombie:
			counts[4]++
		}
	}
	return counts, nil
}
