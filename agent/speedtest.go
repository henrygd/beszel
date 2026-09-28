package agent

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/speedtest"
)

// SpeedtestManager manages scheduled speedtests. Runs are serialized across all
// speedtests, since concurrent tests would compete for the same bandwidth.
type SpeedtestManager struct {
	mu     sync.RWMutex
	tasks  map[string]*speedtestTask // keyed by speedtest ID
	runner speedtestRunner
	// runSlot holds a token while a run is in progress, so runs are serialized
	// and a queued run can give up as soon as its task is canceled.
	runSlot chan struct{}
}

// speedtestTask holds one immutable configuration and its latest result.
type speedtestTask struct {
	config  speedtest.Config
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	result  *speedtest.Result
	running bool
}

func newSpeedtestManager() *SpeedtestManager {
	return newSpeedtestManagerWithRunner(runOoklaSpeedtest)
}

func newSpeedtestManagerWithRunner(runner speedtestRunner) *SpeedtestManager {
	return &SpeedtestManager{tasks: make(map[string]*speedtestTask), runner: runner, runSlot: make(chan struct{}, 1)}
}

func newSpeedtestTask(config speedtest.Config, existing *speedtestTask) *speedtestTask {
	ctx, cancel := context.WithCancel(context.Background())
	task := &speedtestTask{config: config, ctx: ctx, cancel: cancel}
	if existing != nil {
		task.result = existing.latestResult()
	}
	return task
}

// HandleSyncRequest applies a full or incremental speedtest sync request.
func (sm *SpeedtestManager) HandleSyncRequest(req speedtest.SyncRequest) error {
	switch req.Action {
	case monitor.SyncActionReplace:
		sm.SyncSpeedtests(req.Configs)
		return nil
	case monitor.SyncActionUpsert:
		return sm.UpsertSpeedtest(req.Config, req.RunNow)
	case monitor.SyncActionDelete:
		if req.Config.ID == "" {
			return errors.New("missing speedtest ID for delete")
		}
		sm.DeleteSpeedtest(req.Config.ID)
		return nil
	default:
		return fmt.Errorf("unknown speedtest sync action: %d", req.Action)
	}
}

// SyncSpeedtests replaces all speedtest tasks with the given configs.
func (sm *SpeedtestManager) SyncSpeedtests(configs []speedtest.Config) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	newConfigs := make(map[string]speedtest.Config, len(configs))
	for _, cfg := range configs {
		if cfg.ID != "" {
			newConfigs[cfg.ID] = cfg
		}
	}
	for id, task := range sm.tasks {
		if _, exists := newConfigs[id]; !exists {
			task.cancel()
			delete(sm.tasks, id)
		}
	}
	for id, cfg := range newConfigs {
		existing, exists := sm.tasks[id]
		if exists && existing.config == cfg {
			continue
		}
		if exists {
			existing.cancel()
		}
		task := newSpeedtestTask(cfg, existing)
		sm.tasks[id] = task
		sm.schedule(task, false)
	}
}

// UpsertSpeedtest creates or replaces a single speedtest task. If runNow is
// true, a run starts in the background and its result is reported with the next stats.
func (sm *SpeedtestManager) UpsertSpeedtest(config speedtest.Config, runNow bool) error {
	if config.ID == "" {
		return errors.New("missing speedtest ID")
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	existing, exists := sm.tasks[config.ID]
	if exists && existing.config == config {
		if runNow {
			go sm.run(existing)
		}
		return nil
	}
	if exists {
		existing.cancel()
	}
	task := newSpeedtestTask(config, existing)
	sm.tasks[config.ID] = task
	sm.schedule(task, runNow)
	return nil
}

// DeleteSpeedtest stops and removes a single speedtest task.
func (sm *SpeedtestManager) DeleteSpeedtest(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if task, exists := sm.tasks[id]; exists {
		task.cancel()
		delete(sm.tasks, id)
	}
}

// GetResults returns the latest result of each speedtest that has run, or nil if none have.
func (sm *SpeedtestManager) GetResults() map[string]speedtest.Result {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var results map[string]speedtest.Result
	for id, task := range sm.tasks {
		result := task.latestResult()
		if result == nil {
			continue
		}
		if results == nil {
			results = make(map[string]speedtest.Result, len(sm.tasks))
		}
		results[id] = *result
	}
	return results
}

// Stop stops all speedtest tasks, including any run in progress.
func (sm *SpeedtestManager) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for id, task := range sm.tasks {
		task.cancel()
		delete(sm.tasks, id)
	}
}

// schedule starts the task's timer. Runs land on wall-clock slots offset by a
// hash of the speedtest ID, so a speedtest keeps its slot across restarts and
// agents sharing an uplink don't all test at once. With runNow, the first run
// starts immediately and any slot within half an interval of it is skipped.
func (sm *SpeedtestManager) schedule(task *speedtestTask, runNow bool) {
	interval := time.Duration(max(task.config.Interval, speedtest.MinInterval)) * time.Minute
	offset := speedtestSlotOffset(task.config.ID, interval)
	from := time.Now()
	if runNow {
		from = from.Add(interval / 2)
		go sm.run(task)
	}
	slog.Debug("starting speedtest task", "id", task.config.ID, "offset", offset, "interval", interval)
	go runSpeedtestSchedule(task.ctx, interval, offset, from, func() { sm.run(task) })
}

// speedtestSlotOffset returns a stable offset within the interval, in whole
// seconds, derived from the speedtest ID.
func speedtestSlotOffset(id string, interval time.Duration) time.Duration {
	h := fnv.New64a()
	h.Write([]byte(id))
	return time.Duration(h.Sum64()%uint64(interval/time.Second)) * time.Second
}

// nextSpeedtestSlot returns the first slot strictly after t. Slots are
// anchored to the Unix epoch, so they don't depend on when the agent started.
func nextSpeedtestSlot(t time.Time, interval, offset time.Duration) time.Time {
	elapsed := time.Duration(t.UnixNano()) - offset
	return time.Unix(0, int64(elapsed-elapsed%interval+interval+offset))
}

// runSpeedtestSchedule runs at each slot after from. A run that overruns
// later slots skips them rather than building a backlog.
func runSpeedtestSchedule(ctx context.Context, interval, offset time.Duration, from time.Time, run func()) {
	for {
		next := nextSpeedtestSlot(from, interval, offset)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		run()
		// Never reuse a slot, even if the wall clock stepped backwards.
		from = time.Now()
		if from.Before(next) {
			from = next
		}
	}
}

// run performs one speedtest for the task, unless one is already running or queued.
func (sm *SpeedtestManager) run(task *speedtestTask) {
	task.mu.Lock()
	if task.running {
		task.mu.Unlock()
		return
	}
	task.running = true
	task.mu.Unlock()
	defer func() {
		task.mu.Lock()
		task.running = false
		task.mu.Unlock()
	}()

	select {
	case <-task.ctx.Done():
		return
	case sm.runSlot <- struct{}{}:
	}
	defer func() { <-sm.runSlot }()
	if task.ctx.Err() != nil {
		return
	}
	slog.Debug("speedtest started", "id", task.config.ID, "server", task.config.ServerID)
	start := time.Now()
	result, err := sm.runner(task.ctx, task.config.ServerID)
	if task.ctx.Err() != nil {
		return
	}
	if err != nil {
		slog.Warn("speedtest failed", "err", err, "server", task.config.ServerID)
		result = speedtest.Result{ServerID: task.config.ServerID, Error: err.Error()}
	} else {
		slog.Debug("speedtest finished", "id", task.config.ID, "server", result.ServerID, "duration", time.Since(start).Round(time.Millisecond), "download", result.Download, "upload", result.Upload, "ping", result.Ping)
	}
	result.RunAt = time.Now().UnixMilli()
	task.mu.Lock()
	task.result = &result
	task.mu.Unlock()
}

// latestResult returns a copy of the latest result, or nil if the task has not run yet.
func (task *speedtestTask) latestResult() *speedtest.Result {
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.result == nil {
		return nil
	}
	result := *task.result
	return &result
}
