package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"

	// "strconv"
	"sync"
	"time"

	"log/slog"

	"github.com/henrygd/beszel/internal/entities/monitor"
)

// Monitors run at user-defined intervals (e.g., every 10s).
// To keep memory usage low and constant, data is stored in two layers:
// 1. Raw samples: The most recent individual results (kept for monitorRawRetention).
// 2. Minute buckets: A ring buffer of 61 buckets, each representing one
//    wall-clock minute. Samples collected within the same minute are aggregated
//    (sum, min, max, count) into a single bucket.
//
// Short-term requests (<= 70s) use raw samples.
// Long-term requests (up to 1h) use the minute buckets to avoid storing thousands
// of individual data points.

const (
	// monitorRawRetention is the duration to keep individual samples
	monitorRawRetention = 61 * time.Second
	// monitorMinuteBucketLen is the number of 1-minute buckets to keep (1 hour + 1 for partials)
	monitorMinuteBucketLen int32 = 61
)

// MonitorManager manages network monitor tasks.
type MonitorManager struct {
	mu         sync.RWMutex
	monitors   map[string]*monitorTask // key = monitor.Config.Key()
	httpClient *http.Client
}

// monitorTask owns retention buffers and cancellation for a single monitor config.
type monitorTask struct {
	config  monitor.Config
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	samples []monitorSample
	buckets [monitorMinuteBucketLen]monitorBucket
}

// monitorSample stores one monitor attempt and its collection time.
type monitorSample struct {
	responseUs int64 // -1 means loss
	timestamp  time.Time
}

// monitorBucket stores one minute of aggregated monitor data.
type monitorBucket struct {
	minute int32
	filled bool
	stats  monitorAggregate
}

// monitorAggregate accumulates successful response stats and total sample counts.
type monitorAggregate struct {
	sumUs        int64
	minUs        int64
	maxUs        int64
	totalCount   int64
	successCount int64
}

func newMonitorManager() *MonitorManager {
	return &MonitorManager{
		monitors:   make(map[string]*monitorTask),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func newMonitorTask(config monitor.Config) *monitorTask {
	ctx, cancel := context.WithCancel(context.Background())
	return &monitorTask{
		config:  config,
		ctx:     ctx,
		cancel:  cancel,
		samples: make([]monitorSample, 0, 64),
	}
}

func newMonitorTaskFromExisting(config monitor.Config, existing *monitorTask) *monitorTask {
	task := newMonitorTask(config)
	if existing == nil {
		return task
	}

	existing.mu.Lock()
	defer existing.mu.Unlock()
	task.samples = append(task.samples, existing.samples...)
	task.buckets = existing.buckets
	return task
}

// newMonitorAggregate initializes an aggregate with an unset minimum value.
func newMonitorAggregate() monitorAggregate {
	return monitorAggregate{minUs: math.MaxInt64}
}

// addResponse folds a single monitor sample into the aggregate.
func (agg *monitorAggregate) addResponse(responseUs int64) {
	agg.totalCount++
	if responseUs < 0 {
		return
	}
	agg.successCount++
	agg.sumUs += responseUs
	if responseUs < agg.minUs {
		agg.minUs = responseUs
	}
	if responseUs > agg.maxUs {
		agg.maxUs = responseUs
	}
}

// addAggregate merges another aggregate into this one.
func (agg *monitorAggregate) addAggregate(other monitorAggregate) {
	if other.totalCount == 0 {
		return
	}
	agg.totalCount += other.totalCount
	agg.successCount += other.successCount
	agg.sumUs += other.sumUs
	if other.successCount == 0 {
		return
	}
	if agg.minUs == math.MaxInt64 || other.minUs < agg.minUs {
		agg.minUs = other.minUs
	}
	if other.maxUs > agg.maxUs {
		agg.maxUs = other.maxUs
	}
}

// hasData reports whether the aggregate contains any samples.
func (agg monitorAggregate) hasData() bool {
	return agg.totalCount > 0
}

// result converts the aggregate into the monitor result format.
func (agg monitorAggregate) result() monitor.Result {
	avg := agg.avgResponse()
	result := monitor.Result{
		AvgResponse: avg,
		MinResponse: agg.minUs,
		MaxResponse: agg.maxUs,
		PacketLoss:  agg.lossPercentage(),
	}
	if agg.successCount == 0 {
		result.MinResponse, result.MaxResponse = 0, 0
	}
	return result
}

// avgResponse returns the rounded average of successful samples.
func (agg monitorAggregate) avgResponse() int64 {
	if agg.successCount == 0 {
		return 0
	}
	return agg.sumUs / agg.successCount

}

// lossPercentage returns the rounded failure rate for the aggregate.
func (agg monitorAggregate) lossPercentage() float64 {
	if agg.totalCount == 0 {
		return 0
	}
	return math.Round(float64(agg.totalCount-agg.successCount)/float64(agg.totalCount)*10000) / 100
}

// SyncMonitors replaces all monitor tasks with the given configs.
func (pm *MonitorManager) SyncMonitors(configs []monitor.Config) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Build set of new keys
	newKeys := make(map[string]monitor.Config, len(configs))
	for _, cfg := range configs {
		if cfg.ID == "" {
			continue
		}
		newKeys[cfg.ID] = cfg
	}

	// Stop removed monitors
	for key, task := range pm.monitors {
		if _, exists := newKeys[key]; !exists {
			task.cancel()
			delete(pm.monitors, key)
		}
	}

	// Start new monitors and restart tasks whose config changed.
	for key, cfg := range newKeys {
		task, exists := pm.monitors[key]
		if exists && task.config == cfg {
			continue
		}
		if exists {
			task.cancel()
		}
		task = newMonitorTaskFromExisting(cfg, task)
		pm.monitors[key] = task
		go pm.runMonitor(task, false)
	}
}

// HandleSyncRequest applies a full or incremental monitor sync request.
func (pm *MonitorManager) HandleSyncRequest(req monitor.SyncRequest) (monitor.SyncResponse, error) {
	switch req.Action {
	case monitor.SyncActionReplace:
		pm.SyncMonitors(req.Configs)
		return monitor.SyncResponse{}, nil
	case monitor.SyncActionUpsert:
		result, err := pm.UpsertMonitor(req.Config, req.RunNow)
		if err != nil {
			return monitor.SyncResponse{}, err
		}
		if result == nil {
			return monitor.SyncResponse{}, nil
		}
		return monitor.SyncResponse{Result: *result}, nil
	case monitor.SyncActionDelete:
		if req.Config.ID == "" {
			return monitor.SyncResponse{}, errors.New("missing monitor ID for delete")
		}
		pm.DeleteMonitor(req.Config.ID)
		return monitor.SyncResponse{}, nil
	default:
		return monitor.SyncResponse{}, fmt.Errorf("unknown monitor sync action: %d", req.Action)
	}
}

// UpsertMonitor creates or replaces a single monitor task.
func (pm *MonitorManager) UpsertMonitor(config monitor.Config, runNow bool) (*monitor.Result, error) {
	if config.ID == "" {
		return nil, errors.New("missing monitor ID")
	}

	pm.mu.Lock()
	task, exists := pm.monitors[config.ID]
	startTask := false
	if exists && task.config == config {
		pm.mu.Unlock()
		if !runNow {
			return nil, nil
		}
		return pm.runMonitorNow(task), nil
	}
	if exists {
		task.cancel()
	}
	task = newMonitorTaskFromExisting(config, task)
	pm.monitors[config.ID] = task
	startTask = true
	pm.mu.Unlock()

	if runNow {
		result := pm.runMonitorNow(task)
		if startTask {
			go pm.runMonitor(task, false)
		}
		return result, nil
	}
	if startTask {
		go pm.runMonitor(task, false)
	}
	return nil, nil
}

// DeleteMonitor stops and removes a single monitor task.
func (pm *MonitorManager) DeleteMonitor(id string) {
	if id == "" {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if task, exists := pm.monitors[id]; exists {
		task.cancel()
		delete(pm.monitors, id)
	}
}

// GetResults returns aggregated results for all monitors over the last supplied duration in ms.
func (pm *MonitorManager) GetResults(durationMs uint16) map[string]monitor.Result {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	results := make(map[string]monitor.Result, len(pm.monitors))
	now := time.Now()
	duration := time.Duration(durationMs) * time.Millisecond

	for _, task := range pm.monitors {
		task.mu.Lock()
		result, ok := task.resultLocked(duration, now)
		task.mu.Unlock()

		if !ok {
			continue
		}
		results[task.config.ID] = result
	}

	return results
}

// Stop stops all monitor tasks.
func (pm *MonitorManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for key, task := range pm.monitors {
		task.cancel()
		delete(pm.monitors, key)
	}
}

// runMonitor executes a single monitor task in a loop.
func (pm *MonitorManager) runMonitor(task *monitorTask, runNow bool) {
	interval := time.Duration(task.config.Interval) * time.Second
	if interval < time.Second {
		interval = 30 * time.Second
	}

	stagger := getStagger(interval.Milliseconds())

	slog.Debug("starting monitor task", "target", task.config.Target, "delay", stagger.String(), "interval", interval.String())

	if runNow {
		pm.executeMonitor(task)
	}

	select {
	case <-task.ctx.Done():
		// slog.Info("removed monitor", "target", task.config.Target)
		return
	case <-time.After(stagger):
		pm.executeMonitor(task)
	}

	ticker := time.Tick(interval)

	for {
		select {
		case <-task.ctx.Done():
			// slog.Info("removed monitor", "target", task.config.Target)
			return
		case <-ticker:
			pm.executeMonitor(task)
		}
	}
}

// getStagger returns a random duration between intervalSeconds/2 and intervalSeconds to stagger initial monitor executions
func getStagger(intervalMilli int64) time.Duration {
	intervalMilliInt := int(intervalMilli)
	randomDelayInt := rand.Intn(intervalMilliInt)
	if randomDelayInt < intervalMilliInt/2 {
		randomDelayInt += intervalMilliInt / 2
	}
	return time.Duration(randomDelayInt) * time.Millisecond
}

func (pm *MonitorManager) runMonitorNow(task *monitorTask) *monitor.Result {
	pm.executeMonitor(task)
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.ctx.Err() != nil {
		return nil
	}
	result, ok := task.resultLocked(time.Minute, time.Now())
	if !ok {
		return nil
	}
	return &result
}

// resultLocked returns the aggregated monitor result for the requested duration along with a bool indicating whether any data was available.
func (task *monitorTask) resultLocked(duration time.Duration, now time.Time) (monitor.Result, bool) {
	agg := task.aggregateLocked(duration, now)
	if !agg.hasData() {
		// short realtime windows (e.g. the 1s window used for 1m/realtime charts) often fall
		// between monitor samples since monitors run at longer, user-defined intervals; fall back to
		// the most recent sample so realtime requests still report current status.
		agg = task.latestSampleAggregateLocked()
	}
	hourAgg := task.aggregateLocked(time.Hour, now)
	if !agg.hasData() {
		return monitor.Result{}, false
	}

	result := agg.result()

	result.AvgResponse1h = hourAgg.avgResponse()
	result.MinResponse1h = hourAgg.minUs
	result.MaxResponse1h = hourAgg.maxUs
	result.PacketLoss1h = hourAgg.lossPercentage()

	if hourAgg.successCount == 0 {
		result.MinResponse1h, result.MaxResponse1h = 0, 0
	}
	return result, true
}

// latestSampleAggregateLocked returns an aggregate containing only the most recent sample, if any.
func (task *monitorTask) latestSampleAggregateLocked() monitorAggregate {
	agg := newMonitorAggregate()
	if len(task.samples) == 0 {
		return agg
	}
	agg.addResponse(task.samples[len(task.samples)-1].responseUs)
	return agg
}

// aggregateLocked collects monitor data for the requested time window.
func (task *monitorTask) aggregateLocked(duration time.Duration, now time.Time) monitorAggregate {
	cutoff := now.Add(-duration)
	// Keep short windows exact; longer windows read from minute buckets to avoid raw-sample retention.
	if duration <= monitorRawRetention {
		return aggregateSamplesSince(task.samples, cutoff)
	}
	return aggregateBucketsSince(task.buckets[:], cutoff, now)
}

// aggregateSamplesSince aggregates raw samples newer than the cutoff.
func aggregateSamplesSince(samples []monitorSample, cutoff time.Time) monitorAggregate {
	agg := newMonitorAggregate()
	for _, sample := range samples {
		if sample.timestamp.Before(cutoff) {
			continue
		}
		agg.addResponse(sample.responseUs)
	}
	return agg
}

// aggregateBucketsSince aggregates minute buckets overlapping the requested window.
func aggregateBucketsSince(buckets []monitorBucket, cutoff, now time.Time) monitorAggregate {
	agg := newMonitorAggregate()
	startMinute := int32(cutoff.Unix() / 60)
	endMinute := int32(now.Unix() / 60)
	for _, bucket := range buckets {
		if !bucket.filled || bucket.minute < startMinute || bucket.minute > endMinute {
			continue
		}
		agg.addAggregate(bucket.stats)
	}
	return agg
}

// addSampleLocked stores a fresh sample in both raw and per-minute retention buffers.
func (task *monitorTask) addSampleLocked(sample monitorSample) {
	cutoff := sample.timestamp.Add(-monitorRawRetention)
	start := 0
	for i := range task.samples {
		if !task.samples[i].timestamp.Before(cutoff) {
			start = i
			break
		}
		if i == len(task.samples)-1 {
			start = len(task.samples)
		}
	}
	if start > 0 {
		size := copy(task.samples, task.samples[start:])
		task.samples = task.samples[:size]
	}
	task.samples = append(task.samples, sample)

	minute := int32(sample.timestamp.Unix() / 60)
	// Each slot stores one wall-clock minute, so the ring stays fixed-size at ~1h per monitor.
	bucket := &task.buckets[minute%monitorMinuteBucketLen]
	if !bucket.filled || bucket.minute != minute {
		bucket.minute = minute
		bucket.filled = true
		bucket.stats = newMonitorAggregate()
	}
	bucket.stats.addResponse(sample.responseUs)
}

// executeMonitor runs the configured monitor and records the sample.
func (pm *MonitorManager) executeMonitor(task *monitorTask) {
	if task.ctx.Err() != nil {
		return
	}
	var responseUs int64
	var err error

	switch task.config.Protocol {
	case "icmp":
		responseUs, err = monitorICMP(task.ctx, task.config.Target)
	case "tcp":
		responseUs, err = monitorTCP(task.ctx, task.config.Target, task.config.Port)
	case "http":
		responseUs, err = monitorHTTP(task.ctx, pm.httpClient, task.config.Target)
	case "dns":
		responseUs, err = monitorDNS(task.ctx, task.config.Target)
	default:
		slog.Warn("unknown monitor protocol", "protocol", task.config.Protocol)
		return
	}

	// Task cancellation is not a failed network check.
	if task.ctx.Err() != nil {
		return
	}
	if err != nil {
		slog.Warn("monitor failed", "err", err, "target", task.config.Target, "protocol", task.config.Protocol)
	}

	sample := monitorSample{
		responseUs: responseUs,
		timestamp:  time.Now(),
	}

	task.mu.Lock()
	if task.ctx.Err() == nil {
		task.addSampleLocked(sample)
	}
	task.mu.Unlock()
}

// monitorTCP measures pure TCP handshake response (excluding DNS resolution).
// Returns -1 and an error on failure.
func monitorTCP(ctx context.Context, target string, port uint16) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Resolve DNS first, outside the timing window but within the probe deadline.
	ips, err := net.DefaultResolver.LookupHost(ctx, target)
	if err != nil || len(ips) == 0 {
		return -1, err
	}
	addr := net.JoinHostPort(ips[0], fmt.Sprintf("%d", port))

	// Measure only the TCP handshake
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return -1, err
	}
	conn.Close()
	return time.Since(start).Microseconds(), nil
}

// monitorDNS measures DNS resolution response time in microseconds. Returns -1 and an error on failure.
func monitorDNS(ctx context.Context, target string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	start := time.Now()
	ips, err := net.DefaultResolver.LookupHost(ctx, target)
	if err != nil || len(ips) == 0 {
		return -1, err
	}
	return time.Since(start).Microseconds(), nil
}

// monitorHTTP measures HTTP GET request response in microseconds. Returns -1 and an error on failure.
func monitorHTTP(ctx context.Context, client *http.Client, url string) (int64, error) {
	if client == nil {
		client = http.DefaultClient
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return -1, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return -1, fmt.Errorf("HTTP error: %s", resp.Status)
	}
	return time.Since(start).Microseconds(), nil
}
