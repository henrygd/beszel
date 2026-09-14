package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorHistoryAggregateLockedUsesRawSamplesForShortWindows(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	history := newMonitorHistory()

	history.addSampleLocked(monitorSample{responseUs: 10, timestamp: now.Add(-90 * time.Second)})
	history.addSampleLocked(monitorSample{responseUs: 20, timestamp: now.Add(-30 * time.Second)})
	history.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-10 * time.Second)})

	agg := history.aggregateLocked(time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(2), agg.totalCount)
	assert.Equal(t, int64(1), agg.successCount)
	result := agg.result()
	assert.Equal(t, int64(20), result.AvgResponse)
	assert.Equal(t, int64(20), result.MinResponse)
	assert.Equal(t, int64(20), result.MaxResponse)
	assert.Equal(t, 50.0, result.PacketLoss)
}

func TestMonitorHistoryAggregateLockedUsesMinuteBucketsForLongWindows(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 30, 0, time.UTC)
	history := newMonitorHistory()

	history.addSampleLocked(monitorSample{responseUs: 10, timestamp: now.Add(-11 * time.Minute)})
	history.addSampleLocked(monitorSample{responseUs: 20, timestamp: now.Add(-9 * time.Minute)})
	history.addSampleLocked(monitorSample{responseUs: 40, timestamp: now.Add(-5 * time.Minute)})
	history.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-90 * time.Second)})
	history.addSampleLocked(monitorSample{responseUs: 30, timestamp: now.Add(-30 * time.Second)})

	agg := history.aggregateLocked(10*time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(4), agg.totalCount)
	assert.Equal(t, int64(3), agg.successCount)
	result := agg.result()
	assert.Equal(t, int64(30), result.AvgResponse)
	assert.Equal(t, int64(20), result.MinResponse)
	assert.Equal(t, int64(40), result.MaxResponse)
	assert.Equal(t, 25.0, result.PacketLoss)
}

func TestMonitorHistoryAddSampleLockedTrimsRawSamplesButKeepsBucketHistory(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	history := newMonitorHistory()

	history.addSampleLocked(monitorSample{responseUs: 10, timestamp: now.Add(-10 * time.Minute)})
	history.addSampleLocked(monitorSample{responseUs: 20, timestamp: now})

	require.Len(t, history.samples, 1)
	assert.Equal(t, int64(20), history.samples[0].responseUs)

	agg := history.aggregateLocked(10*time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(2), agg.totalCount)
	assert.Equal(t, int64(2), agg.successCount)
	result := agg.result()
	assert.Equal(t, int64(15), result.AvgResponse)
	assert.Equal(t, int64(10), result.MinResponse)
	assert.Equal(t, int64(20), result.MaxResponse)
	assert.Equal(t, 0.0, result.PacketLoss)
}
