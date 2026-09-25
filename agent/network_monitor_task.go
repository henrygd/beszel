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
	resumeGuard    *monitorResumeGuard
	runMu          sync.Mutex
	inflight       *monitorRun
	lastFailureLog int64 // Unix nanoseconds

	certMu        sync.Mutex
	cert          *monitor.CertInfo
	certUnsent    bool // cert has not been included in a stats result yet
	certChecking  bool
	nextCertCheck time.Time
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
		// Keep the last known certificate, but check again soon for the new config.
		// The hub already stores it, so it is not marked unsent.
		if config.Target == existing.config.Target {
			task.cert = existing.certInfo()
		}
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

	generation, _ := task.resumeGuard.snapshot()
	responseUs, pings, err := task.check(probe)
	var logFailure bool
	task.runMu.Lock()
	currentGeneration, _ := task.resumeGuard.snapshot()
	if task.ctx.Err() == nil && generation == currentGeneration {
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
		result := task.history.record(monitorSample{responseUs: responseUs, timestamp: now, pings: pings})
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

// check runs one probe, or Count concurrent probes for a multi-ping ICMP config.
// Multi-ping checks return per-ping stats so partial loss is reflected, and only
// report an error when every ping failed.
func (task *monitorTask) check(probe monitorProbe) (int64, *monitorAggregate, error) {
	config := task.config
	if config.Protocol != "icmp" || config.Count <= 1 {
		responseUs, err := probe(task.ctx, config)
		return responseUs, nil, err
	}
	count := min(int(config.Count), monitor.MaxICMPCount)
	type pingResult struct {
		responseUs int64
		err        error
	}
	results := make([]pingResult, count)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			results[i].responseUs, results[i].err = probe(task.ctx, config)
		})
	}
	wg.Wait()

	agg := newMonitorAggregate()
	var firstErr error
	for _, r := range results {
		if r.err != nil {
			r.responseUs = -1
			if firstErr == nil {
				firstErr = r.err
			}
		}
		agg.addResponse(r.responseUs)
	}
	if agg.successCount == 0 {
		return -1, &agg, firstErr
	}
	return agg.avgResponse(), &agg, nil
}

// refreshCert checks the certificate of an HTTPS target when due. A failed
// check keeps the last known certificate and retries sooner, as does a
// certificate that expires before the next regular check, so renewals show up
// quickly. Concurrent callers skip rather than wait, and no lock is held during
// network I/O.
func (task *monitorTask) refreshCert(check certChecker) {
	if check == nil || !certCheckEnabled(task.config) {
		return
	}
	task.certMu.Lock()
	if task.certChecking || time.Now().Before(task.nextCertCheck) {
		task.certMu.Unlock()
		return
	}
	task.certChecking = true
	task.certMu.Unlock()

	info, err := check(task.ctx, task.config.Target)

	task.certMu.Lock()
	defer task.certMu.Unlock()
	task.certChecking = false
	if task.ctx.Err() != nil {
		return
	}
	if err != nil {
		task.nextCertCheck = time.Now().Add(certCheckRetryInterval)
		slog.Warn("certificate check failed", "err", err, "target", task.config.Target)
		return
	}
	task.cert = &info
	task.certUnsent = true
	now := time.Now()
	interval := certCheckInterval
	if time.UnixMilli(info.Expires).Before(now.Add(certCheckInterval)) {
		interval = certCheckRetryInterval
	}
	task.nextCertCheck = now.Add(interval)
}

// certInfo returns a copy of the latest certificate info, or nil if unknown.
func (task *monitorTask) certInfo() *monitor.CertInfo {
	task.certMu.Lock()
	defer task.certMu.Unlock()
	if task.cert == nil {
		return nil
	}
	cert := *task.cert
	return &cert
}

// takeUnsentCert returns the latest certificate info once after each successful
// check, so unchanged info is not resent with every stats result.
func (task *monitorTask) takeUnsentCert() *monitor.CertInfo {
	task.certMu.Lock()
	defer task.certMu.Unlock()
	if !task.certUnsent {
		return nil
	}
	task.certUnsent = false
	cert := *task.cert
	return &cert
}

func copyMonitorResult(result *monitor.Result) *monitor.Result {
	if result == nil {
		return nil
	}
	copy := *result
	return &copy
}
