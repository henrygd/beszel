package agent

import (
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

	"github.com/henrygd/beszel/internal/entities/probe"
)

// Probe functionality overview:
// Probes run at user-defined intervals (e.g., every 10s).
// To keep memory usage low and constant, data is stored in two layers:
// 1. Raw samples: The most recent individual results (kept for probeRawRetention).
// 2. Minute buckets: A fixed-size ring buffer of 61 buckets, each representing one
//    wall-clock minute. Samples collected within the same minute are aggregated
//    (sum, min, max, count) into a single bucket.
//
// Short-term requests (<= 2m) use raw samples for perfect accuracy.
// Long-term requests (up to 1h) use the minute buckets to avoid storing thousands
// of individual data points.

const (
	// probeRawRetention is the duration to keep individual samples for high-precision short-term requests
	probeRawRetention = 70 * time.Second
	// probeMinuteBucketLen is the number of 1-minute buckets to keep (1 hour + 1 for partials)
	probeMinuteBucketLen int32 = 61
)

// ProbeManager manages network probe tasks.
type ProbeManager struct {
	mu         sync.RWMutex
	probes     map[string]*probeTask // key = probe.Config.Key()
	httpClient *http.Client
}

// probeTask owns retention buffers and cancellation for a single probe config.
type probeTask struct {
	config  probe.Config
	cancel  chan struct{}
	mu      sync.Mutex
	samples []probeSample
	buckets [probeMinuteBucketLen]probeBucket
}

// probeSample stores one probe attempt and its collection time.
type probeSample struct {
	responseUs int64 // -1 means loss
	timestamp  time.Time
}

// probeBucket stores one minute of aggregated probe data.
type probeBucket struct {
	minute int32
	filled bool
	stats  probeAggregate
}

// probeAggregate accumulates successful response stats and total sample counts.
type probeAggregate struct {
	sumUs        int64
	minUs        int64
	maxUs        int64
	totalCount   int64
	successCount int64
}

func newProbeManager() *ProbeManager {
	return &ProbeManager{
		probes:     make(map[string]*probeTask),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func newProbeTask(config probe.Config) *probeTask {
	return &probeTask{
		config:  config,
		cancel:  make(chan struct{}),
		samples: make([]probeSample, 0, 64),
	}
}

func newProbeTaskFromExisting(config probe.Config, existing *probeTask) *probeTask {
	task := newProbeTask(config)
	if existing == nil {
		return task
	}

	existing.mu.Lock()
	defer existing.mu.Unlock()
	task.samples = append(task.samples, existing.samples...)
	task.buckets = existing.buckets
	return task
}

// newProbeAggregate initializes an aggregate with an unset minimum value.
func newProbeAggregate() probeAggregate {
	return probeAggregate{minUs: math.MaxInt64}
}

// addResponse folds a single probe sample into the aggregate.
func (agg *probeAggregate) addResponse(responseUs int64) {
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
func (agg *probeAggregate) addAggregate(other probeAggregate) {
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
func (agg probeAggregate) hasData() bool {
	return agg.totalCount > 0
}

// result converts the aggregate into the probe result format.
func (agg probeAggregate) result() probe.Result {
	avg := agg.avgResponse()
	result := probe.Result{
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
func (agg probeAggregate) avgResponse() int64 {
	if agg.successCount == 0 {
		return 0
	}
	return agg.sumUs / agg.successCount

}

// lossPercentage returns the rounded failure rate for the aggregate.
func (agg probeAggregate) lossPercentage() float64 {
	if agg.totalCount == 0 {
		return 0
	}
	return math.Round(float64(agg.totalCount-agg.successCount)/float64(agg.totalCount)*10000) / 100
}

// SyncProbes replaces all probe tasks with the given configs.
func (pm *ProbeManager) SyncProbes(configs []probe.Config) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Build set of new keys
	newKeys := make(map[string]probe.Config, len(configs))
	for _, cfg := range configs {
		if cfg.ID == "" {
			continue
		}
		newKeys[cfg.ID] = cfg
	}

	// Stop removed probes
	for key, task := range pm.probes {
		if _, exists := newKeys[key]; !exists {
			close(task.cancel)
			delete(pm.probes, key)
		}
	}

	// Start new probes and restart tasks whose config changed.
	for key, cfg := range newKeys {
		task, exists := pm.probes[key]
		if exists && task.config == cfg {
			continue
		}
		if exists {
			close(task.cancel)
		}
		task = newProbeTaskFromExisting(cfg, task)
		pm.probes[key] = task
		go pm.runProbe(task, false)
	}
}

// HandleSyncRequest applies a full or incremental probe sync request.
func (pm *ProbeManager) HandleSyncRequest(req probe.SyncRequest) (probe.SyncResponse, error) {
	switch req.Action {
	case probe.SyncActionReplace:
		pm.SyncProbes(req.Configs)
		return probe.SyncResponse{}, nil
	case probe.SyncActionUpsert:
		result, err := pm.UpsertProbe(req.Config, req.RunNow)
		if err != nil {
			return probe.SyncResponse{}, err
		}
		if result == nil {
			return probe.SyncResponse{}, nil
		}
		return probe.SyncResponse{Result: *result}, nil
	case probe.SyncActionDelete:
		if req.Config.ID == "" {
			return probe.SyncResponse{}, errors.New("missing probe ID for delete")
		}
		pm.DeleteProbe(req.Config.ID)
		return probe.SyncResponse{}, nil
	default:
		return probe.SyncResponse{}, fmt.Errorf("unknown probe sync action: %d", req.Action)
	}
}

// UpsertProbe creates or replaces a single probe task.
func (pm *ProbeManager) UpsertProbe(config probe.Config, runNow bool) (*probe.Result, error) {
	if config.ID == "" {
		return nil, errors.New("missing probe ID")
	}

	pm.mu.Lock()
	task, exists := pm.probes[config.ID]
	startTask := false
	if exists && task.config == config {
		pm.mu.Unlock()
		if !runNow {
			return nil, nil
		}
		return pm.runProbeNow(task), nil
	}
	if exists {
		close(task.cancel)
	}
	task = newProbeTaskFromExisting(config, task)
	pm.probes[config.ID] = task
	startTask = true
	pm.mu.Unlock()

	if runNow {
		result := pm.runProbeNow(task)
		if startTask {
			go pm.runProbe(task, false)
		}
		return result, nil
	}
	if startTask {
		go pm.runProbe(task, false)
	}
	return nil, nil
}

// DeleteProbe stops and removes a single probe task.
func (pm *ProbeManager) DeleteProbe(id string) {
	if id == "" {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if task, exists := pm.probes[id]; exists {
		close(task.cancel)
		delete(pm.probes, id)
	}
}

// GetResults returns aggregated results for all probes over the last supplied duration in ms.
func (pm *ProbeManager) GetResults(durationMs uint16) map[string]probe.Result {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	results := make(map[string]probe.Result, len(pm.probes))
	now := time.Now()
	duration := time.Duration(durationMs) * time.Millisecond

	for _, task := range pm.probes {
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

// Stop stops all probe tasks.
func (pm *ProbeManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for key, task := range pm.probes {
		close(task.cancel)
		delete(pm.probes, key)
	}
}

// runProbe executes a single probe task in a loop.
func (pm *ProbeManager) runProbe(task *probeTask, runNow bool) {
	interval := time.Duration(task.config.Interval) * time.Second
	if interval < time.Second {
		interval = 30 * time.Second
	}

	stagger := getStagger(interval.Milliseconds())
	slog.Info("starting probe task", "id", task.config.ID, "initial_delay", stagger.String(), "interval", interval.String())

	if runNow {
		pm.executeProbe(task)
	}

	select {
	case <-task.cancel:
		slog.Info("removed probe", "id", task.config.ID)
		return
	case <-time.After(stagger):
		slog.Info("initial probe execution", "id", task.config.ID)
		pm.executeProbe(task)
	}

	ticker := time.Tick(interval)

	for {
		select {
		case <-task.cancel:
			slog.Info("removed probe", "id", task.config.ID)
			return
		case <-ticker:
			slog.Info("running probe in main loop", "id", task.config.ID, "interval", interval.String())
			pm.executeProbe(task)
		}
	}
}

// getStagger returns a random duration between intervalSeconds/2 and intervalSeconds to stagger probe executions
func getStagger(intervalMilli int64) time.Duration {
	intervalMilliInt := int(intervalMilli)
	randomDelayInt := rand.Intn(intervalMilliInt)
	if randomDelayInt < intervalMilliInt/2 {
		randomDelayInt += intervalMilliInt / 2
	}
	return time.Duration(randomDelayInt) * time.Millisecond
}

func (pm *ProbeManager) runProbeNow(task *probeTask) *probe.Result {
	pm.executeProbe(task)
	task.mu.Lock()
	defer task.mu.Unlock()
	result, ok := task.resultLocked(time.Minute, time.Now())
	if !ok {
		return nil
	}
	return &result
}

// aggregateLocked collects probe data for the requested time window.
func (task *probeTask) aggregateLocked(duration time.Duration, now time.Time) probeAggregate {
	cutoff := now.Add(-duration)
	// Keep short windows exact; longer windows read from minute buckets to avoid raw-sample retention.
	if duration <= probeRawRetention {
		return aggregateSamplesSince(task.samples, cutoff)
	}
	return aggregateBucketsSince(task.buckets[:], cutoff, now)
}

// resultLocked returns the aggregated probe result for the requested duration along with a bool indicating whether any data was available.
func (task *probeTask) resultLocked(duration time.Duration, now time.Time) (probe.Result, bool) {
	agg := task.aggregateLocked(duration, now)
	hourAgg := task.aggregateLocked(time.Hour, now)
	if !agg.hasData() {
		return probe.Result{}, false
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

// aggregateSamplesSince aggregates raw samples newer than the cutoff.
func aggregateSamplesSince(samples []probeSample, cutoff time.Time) probeAggregate {
	agg := newProbeAggregate()
	for _, sample := range samples {
		if sample.timestamp.Before(cutoff) {
			continue
		}
		agg.addResponse(sample.responseUs)
	}
	return agg
}

// aggregateBucketsSince aggregates minute buckets overlapping the requested window.
func aggregateBucketsSince(buckets []probeBucket, cutoff, now time.Time) probeAggregate {
	agg := newProbeAggregate()
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
func (task *probeTask) addSampleLocked(sample probeSample) {
	cutoff := sample.timestamp.Add(-probeRawRetention)
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
	// Each slot stores one wall-clock minute, so the ring stays fixed-size at ~1h per probe.
	bucket := &task.buckets[minute%probeMinuteBucketLen]
	if !bucket.filled || bucket.minute != minute {
		bucket.minute = minute
		bucket.filled = true
		bucket.stats = newProbeAggregate()
	}
	bucket.stats.addResponse(sample.responseUs)
}

// executeProbe runs the configured probe and records the sample.
func (pm *ProbeManager) executeProbe(task *probeTask) {
	var responseUs int64

	switch task.config.Protocol {
	case "icmp":
		responseUs = probeICMP(task.config.Target)
	case "tcp":
		responseUs = probeTCP(task.config.Target, task.config.Port)
	case "http":
		responseUs = probeHTTP(pm.httpClient, task.config.Target)
	default:
		slog.Warn("unknown probe protocol", "protocol", task.config.Protocol)
		return
	}

	sample := probeSample{
		responseUs: responseUs,
		timestamp:  time.Now(),
	}

	task.mu.Lock()
	task.addSampleLocked(sample)
	task.mu.Unlock()
}

// probeTCP measures pure TCP handshake response (excluding DNS resolution).
// Returns -1 on failure.
func probeTCP(target string, port uint16) int64 {
	// Resolve DNS first, outside the timing window
	ips, err := net.LookupHost(target)
	if err != nil || len(ips) == 0 {
		return -1
	}
	addr := net.JoinHostPort(ips[0], fmt.Sprintf("%d", port))

	// Measure only the TCP handshake
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return -1
	}
	conn.Close()
	return time.Since(start).Microseconds()
}

// probeHTTP measures HTTP GET request response in microseconds. Returns -1 on failure.
func probeHTTP(client *http.Client, url string) int64 {
	if client == nil {
		client = http.DefaultClient
	}
	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		return -1
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return -1
	}
	return time.Since(start).Microseconds()
}
