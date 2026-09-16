package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
)

const monitorFailureLogInterval = 5 * time.Minute

// monitorTask coordinates a probe and its history for one immutable configuration.
type monitorTask struct {
	config         monitor.Config
	ctx            context.Context
	cancel         context.CancelFunc
	history        *monitorHistory
	runMu          sync.Mutex
	inflight       *monitorRun
	lastFailureLog int64 // Unix nanoseconds
}

type monitorRun struct {
	done   chan struct{}
	result *monitor.Result // published by closing done; never mutated afterwards
}

func newMonitorTask(config monitor.Config) *monitorTask {
	ctx, cancel := context.WithCancel(context.Background())
	task := &monitorTask{config: config, ctx: ctx, history: newMonitorHistory()}
	// Serialize cancellation with publication, so canceled probes cannot enter
	// history copied into a replacement task.
	task.cancel = func() {
		task.runMu.Lock()
		cancel()
		task.runMu.Unlock()
	}
	return task
}

func newMonitorTaskFromExisting(config monitor.Config, existing *monitorTask) *monitorTask {
	task := newMonitorTask(config)
	if existing != nil {
		task.history = existing.history.clone()
	}
	return task
}

// runProbe shares an in-flight check between scheduled and immediate requests.
// Every completed check contributes exactly one sample, regardless of how many
// callers were waiting for it. No task or history lock is held during network I/O.
func (task *monitorTask) runProbe(probe monitorProbe) *monitor.Result {
	task.runMu.Lock()
	if task.ctx.Err() != nil {
		task.runMu.Unlock()
		return nil
	}
	if run := task.inflight; run != nil {
		task.runMu.Unlock()
		select {
		case <-task.ctx.Done():
			return nil
		case <-run.done:
			if task.ctx.Err() != nil {
				return nil
			}
			return copyMonitorResult(run.result)
		}
	}
	run := &monitorRun{done: make(chan struct{})}
	task.inflight = run
	task.runMu.Unlock()

	responseUs, err := probe(task.ctx, task.config)
	var logFailure bool
	task.runMu.Lock()
	if task.ctx.Err() == nil {
		now := time.Now()
		if err != nil {
			responseUs = -1
			logAt := now.UnixNano()
			if task.lastFailureLog == 0 || logAt < task.lastFailureLog || logAt-task.lastFailureLog >= int64(monitorFailureLogInterval) {
				logFailure = true
				task.lastFailureLog = logAt
			}
		} else {
			task.lastFailureLog = 0
		}
		result := task.history.record(monitorSample{responseUs: responseUs, timestamp: now})
		run.result = &result
	}

	task.inflight = nil
	close(run.done)
	task.runMu.Unlock()
	if logFailure {
		slog.Warn("monitor failed", "err", err, "target", task.config.Target, "protocol", task.config.Protocol)
	}
	if task.ctx.Err() != nil {
		return nil
	}
	return copyMonitorResult(run.result)
}

func copyMonitorResult(result *monitor.Result) *monitor.Result {
	if result == nil {
		return nil
	}
	copy := *result
	return &copy
}
