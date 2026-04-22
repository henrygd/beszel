package agent

import (
	"fmt"
	"math"
	"net"
	"net/http"
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
	probeRawRetention = 80 * time.Second
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
	responseMs float64 // -1 means loss
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
	sumMs        float64
	minMs        float64
	maxMs        float64
	totalCount   int
	successCount int
}

func newProbeManager() *ProbeManager {
	return &ProbeManager{
		probes:     make(map[string]*probeTask),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// newProbeAggregate initializes an aggregate with an unset minimum value.
func newProbeAggregate() probeAggregate {
	return probeAggregate{minMs: math.MaxFloat64}
}

// addResponse folds a single probe sample into the aggregate.
func (agg *probeAggregate) addResponse(responseMs float64) {
	agg.totalCount++
	if responseMs < 0 {
		return
	}
	agg.successCount++
	agg.sumMs += responseMs
	if responseMs < agg.minMs {
		agg.minMs = responseMs
	}
	if responseMs > agg.maxMs {
		agg.maxMs = responseMs
	}
}

// addAggregate merges another aggregate into this one.
func (agg *probeAggregate) addAggregate(other probeAggregate) {
	if other.totalCount == 0 {
		return
	}
	agg.totalCount += other.totalCount
	agg.successCount += other.successCount
	agg.sumMs += other.sumMs
	if other.successCount == 0 {
		return
	}
	if agg.minMs == math.MaxFloat64 || other.minMs < agg.minMs {
		agg.minMs = other.minMs
	}
	if other.maxMs > agg.maxMs {
		agg.maxMs = other.maxMs
	}
}

// hasData reports whether the aggregate contains any samples.
func (agg probeAggregate) hasData() bool {
	return agg.totalCount > 0
}

// result converts the aggregate into the probe result slice format.
func (agg probeAggregate) result() probe.Result {
	avg := agg.avgResponse()
	minMs := 0.0
	if agg.successCount > 0 {
		minMs = math.Round(agg.minMs*100) / 100
	}
	return probe.Result{
		avg,
		minMs,
		math.Round(agg.maxMs*100) / 100,
		agg.lossPercentage(),
	}
}

// avgResponse returns the rounded average of successful samples.
func (agg probeAggregate) avgResponse() float64 {
	if agg.successCount == 0 {
		return 0
	}
	return math.Round(agg.sumMs/float64(agg.successCount)*100) / 100
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
		newKeys[cfg.Key()] = cfg
	}

	// Stop removed probes
	for key, task := range pm.probes {
		if _, exists := newKeys[key]; !exists {
			close(task.cancel)
			delete(pm.probes, key)
		}
	}

	// Start new probes (skip existing ones with same key)
	for key, cfg := range newKeys {
		if _, exists := pm.probes[key]; exists {
			continue
		}
		task := &probeTask{
			config:  cfg,
			cancel:  make(chan struct{}),
			samples: make([]probeSample, 0, 64),
		}
		pm.probes[key] = task
		go pm.runProbe(task)
	}
}

// GetResults returns aggregated results for all probes over the last supplied duration in ms.
func (pm *ProbeManager) GetResults(durationMs uint16) map[string]probe.Result {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	results := make(map[string]probe.Result, len(pm.probes))
	now := time.Now()
	duration := time.Duration(durationMs) * time.Millisecond

	for key, task := range pm.probes {
		task.mu.Lock()
		agg := task.aggregateLocked(duration, now)
		// The live request window still controls avg/loss, but the range fields are always 1h.
		hourAgg := task.aggregateLocked(time.Hour, now)
		task.mu.Unlock()

		if !agg.hasData() {
			continue
		}

		result := agg.result()
		hourAvg := hourAgg.avgResponse()
		hourLoss := hourAgg.lossPercentage()
		if hourAgg.successCount > 0 {
			result = probe.Result{
				result[0],
				hourAvg,
				math.Round(hourAgg.minMs*100) / 100,
				math.Round(hourAgg.maxMs*100) / 100,
				hourLoss,
			}
		} else {
			result = probe.Result{result[0], hourAvg, 0, 0, hourLoss}
		}
		results[key] = result
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
func (pm *ProbeManager) runProbe(task *probeTask) {
	interval := time.Duration(task.config.Interval) * time.Second
	if interval < time.Second {
		interval = 10 * time.Second
	}
	ticker := time.Tick(interval)

	// Run immediately on start
	pm.executeProbe(task)

	for {
		select {
		case <-task.cancel:
			return
		case <-ticker:
			pm.executeProbe(task)
		}
	}
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

// aggregateSamplesSince aggregates raw samples newer than the cutoff.
func aggregateSamplesSince(samples []probeSample, cutoff time.Time) probeAggregate {
	agg := newProbeAggregate()
	for _, sample := range samples {
		if sample.timestamp.Before(cutoff) {
			continue
		}
		agg.addResponse(sample.responseMs)
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
	bucket.stats.addResponse(sample.responseMs)
}

// executeProbe runs the configured probe and records the sample.
func (pm *ProbeManager) executeProbe(task *probeTask) {
	var responseMs float64

	switch task.config.Protocol {
	case "icmp":
		responseMs = probeICMP(task.config.Target)
	case "tcp":
		responseMs = probeTCP(task.config.Target, task.config.Port)
	case "http":
		responseMs = probeHTTP(pm.httpClient, task.config.Target)
	default:
		slog.Warn("unknown probe protocol", "protocol", task.config.Protocol)
		return
	}

	sample := probeSample{
		responseMs: responseMs,
		timestamp:  time.Now(),
	}

	task.mu.Lock()
	task.addSampleLocked(sample)
	task.mu.Unlock()
}

// probeTCP measures pure TCP handshake response (excluding DNS resolution).
// Returns -1 on failure.
func probeTCP(target string, port uint16) float64 {
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
	return float64(time.Since(start).Microseconds()) / 1000.0
}

// probeHTTP measures HTTP GET request response. Returns -1 on failure.
func probeHTTP(client *http.Client, url string) float64 {
	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		return -1
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return -1
	}
	return float64(time.Since(start).Microseconds()) / 1000.0
}
