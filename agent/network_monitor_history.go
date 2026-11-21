package agent

import (
	"math"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
)

// Monitors run at user-defined intervals (e.g., every 10s).
// To keep memory usage low and constant, data is stored in two layers:
// 1. Raw samples: The most recent individual results (kept for monitorRawRetention).
// 2. Minute buckets: A ring buffer of 61 buckets, each representing one
//    wall-clock minute. Samples collected within the same minute are aggregated
//    (sum, min, max, count) into a single bucket.
//
// Short-term requests (<= 61s) use raw samples.
// Long-term requests (up to 1h) use the minute buckets to avoid storing thousands
// of individual data points.

const (
	// monitorRawRetention is the duration to keep individual samples
	monitorRawRetention = 61 * time.Second
	// monitorMinuteBucketLen is the number of 1-minute buckets to keep (1 hour + 1 for partials)
	monitorMinuteBucketLen int32 = 61
)

// monitorHistory owns retention and aggregation, independently of probe execution.
type monitorHistory struct {
	mu          sync.Mutex
	sampleCount int64
	samples     []monitorSample
	buckets     [monitorMinuteBucketLen]monitorBucket
}

func newMonitorHistory() *monitorHistory {
	// Start small for typical intervals; append grows the buffer for faster probes.
	return &monitorHistory{samples: make([]monitorSample, 0, 4)}
}

func (h *monitorHistory) clone() *monitorHistory {
	h.mu.Lock()
	defer h.mu.Unlock()
	cloned := newMonitorHistory()
	cloned.samples = append(cloned.samples, h.samples...)
	cloned.buckets = h.buckets
	cloned.sampleCount = h.sampleCount
	return cloned
}

func (h *monitorHistory) result(duration time.Duration, now time.Time) (monitor.Result, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.resultLocked(duration, now)
}

func (h *monitorHistory) record(sample monitorSample) monitor.Result {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.addSampleLocked(sample)
	result, _ := h.resultLocked(time.Minute, sample.timestamp)
	return result
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
		AvgResponse:  avg,
		MinResponse:  agg.minUs,
		MaxResponse:  agg.maxUs,
		PacketLoss:   agg.lossPercentage(),
		TotalCount:   agg.totalCount,
		SuccessCount: agg.successCount,
		ResponseSum:  agg.sumUs,
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

// resultLocked returns the aggregated monitor result for the requested duration along with a bool indicating whether any data was available.
func (h *monitorHistory) resultLocked(duration time.Duration, now time.Time) (monitor.Result, bool) {
	agg := h.aggregateLocked(duration, now)
	if !agg.hasData() {
		// short realtime windows (e.g. the 1s window used for 1m/realtime charts) often fall
		// between monitor samples since monitors run at longer, user-defined intervals; fall back to
		// the most recent sample so realtime requests still report current status.
		agg = h.latestSampleAggregateLocked()
	}
	hourAgg := h.aggregateLocked(time.Hour, now)
	if !agg.hasData() {
		return monitor.Result{}, false
	}

	result := agg.result()
	if len(h.samples) > 0 {
		result.LastProbeAt = h.samples[len(h.samples)-1].timestamp.UnixMilli()
	}

	result.AvgResponse1h = hourAgg.avgResponse()
	result.MinResponse1h = hourAgg.minUs
	result.MaxResponse1h = hourAgg.maxUs
	result.PacketLoss1h = hourAgg.lossPercentage()
	result.SampleCount = h.sampleCount

	if hourAgg.successCount == 0 {
		result.MinResponse1h, result.MaxResponse1h = 0, 0
	}
	return result, true
}

// latestSampleAggregateLocked returns an aggregate containing only the most recent sample, if any.
func (h *monitorHistory) latestSampleAggregateLocked() monitorAggregate {
	agg := newMonitorAggregate()
	if len(h.samples) == 0 {
		return agg
	}
	agg.addResponse(h.samples[len(h.samples)-1].responseUs)
	return agg
}

// aggregateLocked collects monitor data for the requested time window.
func (h *monitorHistory) aggregateLocked(duration time.Duration, now time.Time) monitorAggregate {
	cutoff := now.Add(-duration)
	// Keep short windows exact; longer windows read from minute buckets to avoid raw-sample retention.
	if duration <= monitorRawRetention {
		return aggregateSamplesSince(h.samples, cutoff)
	}
	return aggregateBucketsSince(h.buckets[:], cutoff, now)
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
func (h *monitorHistory) addSampleLocked(sample monitorSample) {
	h.sampleCount++
	cutoff := sample.timestamp.Add(-monitorRawRetention)
	start := 0
	for i := range h.samples {
		if !h.samples[i].timestamp.Before(cutoff) {
			start = i
			break
		}
		if i == len(h.samples)-1 {
			start = len(h.samples)
		}
	}
	if start > 0 {
		size := copy(h.samples, h.samples[start:])
		h.samples = h.samples[:size]
	}
	h.samples = append(h.samples, sample)

	minute := int32(sample.timestamp.Unix() / 60)
	// Each slot stores one wall-clock minute, so the ring stays fixed-size at ~1h per monitor.
	bucket := &h.buckets[minute%monitorMinuteBucketLen]
	if !bucket.filled || bucket.minute != minute {
		bucket.minute = minute
		bucket.filled = true
		bucket.stats = newMonitorAggregate()
	}
	bucket.stats.addResponse(sample.responseUs)
}
