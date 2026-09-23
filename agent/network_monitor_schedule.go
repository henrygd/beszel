package agent

import (
	"context"
	"log/slog"
	"math/rand"
	"time"
)

func (pm *MonitorManager) startMonitor(task *monitorTask) {
	interval := time.Duration(task.config.Interval) * time.Second
	if interval < time.Second {
		interval = 30 * time.Second
	}
	delay := getStagger(interval.Milliseconds())
	slog.Debug("starting monitor task", "target", task.config.Target, "delay", delay, "interval", interval)
	go runMonitorSchedule(task.ctx, interval, delay, func() {
		if _, allowed := task.resumeGuard.snapshot(); allowed {
			task.runProbe(pm.probe)
		}
	})
}

// runMonitorSchedule owns only timing. Checks run serially, and slow checks
// naturally drop missed ticks rather than building an execution backlog.
func runMonitorSchedule(ctx context.Context, interval, delay time.Duration, run func()) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	if ctx.Err() != nil {
		return
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			run()
		}
	}
}

// getStagger returns an initial delay between half an interval and one interval.
func getStagger(intervalMilli int64) time.Duration {
	delay := rand.Intn(int(intervalMilli))
	if delay < int(intervalMilli)/2 {
		delay += int(intervalMilli) / 2
	}
	return time.Duration(delay) * time.Millisecond
}
