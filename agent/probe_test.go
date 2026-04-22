package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeTaskAggregateLockedUsesRawSamplesForShortWindows(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	task := &probeTask{}

	task.addSampleLocked(probeSample{responseMs: 10, timestamp: now.Add(-90 * time.Second)})
	task.addSampleLocked(probeSample{responseMs: 20, timestamp: now.Add(-30 * time.Second)})
	task.addSampleLocked(probeSample{responseMs: -1, timestamp: now.Add(-10 * time.Second)})

	agg := task.aggregateLocked(time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, 2, agg.totalCount)
	assert.Equal(t, 1, agg.successCount)
	assert.Equal(t, 20.0, agg.result()[0])
	assert.Equal(t, 20.0, agg.result()[1])
	assert.Equal(t, 20.0, agg.result()[2])
	assert.Equal(t, 50.0, agg.result()[3])
}

func TestProbeTaskAggregateLockedUsesMinuteBucketsForLongWindows(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 30, 0, time.UTC)
	task := &probeTask{}

	task.addSampleLocked(probeSample{responseMs: 10, timestamp: now.Add(-11 * time.Minute)})
	task.addSampleLocked(probeSample{responseMs: 20, timestamp: now.Add(-9 * time.Minute)})
	task.addSampleLocked(probeSample{responseMs: 40, timestamp: now.Add(-5 * time.Minute)})
	task.addSampleLocked(probeSample{responseMs: -1, timestamp: now.Add(-90 * time.Second)})
	task.addSampleLocked(probeSample{responseMs: 30, timestamp: now.Add(-30 * time.Second)})

	agg := task.aggregateLocked(10*time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, 4, agg.totalCount)
	assert.Equal(t, 3, agg.successCount)
	assert.Equal(t, 30.0, agg.result()[0])
	assert.Equal(t, 20.0, agg.result()[1])
	assert.Equal(t, 40.0, agg.result()[2])
	assert.Equal(t, 25.0, agg.result()[3])
}

func TestProbeTaskAddSampleLockedTrimsRawSamplesButKeepsBucketHistory(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	task := &probeTask{}

	task.addSampleLocked(probeSample{responseMs: 10, timestamp: now.Add(-10 * time.Minute)})
	task.addSampleLocked(probeSample{responseMs: 20, timestamp: now})

	require.Len(t, task.samples, 1)
	assert.Equal(t, 20.0, task.samples[0].responseMs)

	agg := task.aggregateLocked(10*time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, 2, agg.totalCount)
	assert.Equal(t, 2, agg.successCount)
	assert.Equal(t, 15.0, agg.result()[0])
	assert.Equal(t, 10.0, agg.result()[1])
	assert.Equal(t, 20.0, agg.result()[2])
	assert.Equal(t, 0.0, agg.result()[3])
}

func TestProbeManagerGetResultsIncludesHourResponseRange(t *testing.T) {
	now := time.Now().UTC()
	task := &probeTask{}
	task.addSampleLocked(probeSample{responseMs: 10, timestamp: now.Add(-30 * time.Minute)})
	task.addSampleLocked(probeSample{responseMs: 20, timestamp: now.Add(-9 * time.Minute)})
	task.addSampleLocked(probeSample{responseMs: 40, timestamp: now.Add(-5 * time.Minute)})
	task.addSampleLocked(probeSample{responseMs: -1, timestamp: now.Add(-90 * time.Second)})
	task.addSampleLocked(probeSample{responseMs: 30, timestamp: now.Add(-30 * time.Second)})

	pm := &ProbeManager{probes: map[string]*probeTask{"icmp:example.com": task}}

	results := pm.GetResults(uint16(time.Minute / time.Millisecond))
	result, ok := results["icmp:example.com"]
	require.True(t, ok)
	require.Len(t, result, 5)
	assert.Equal(t, 30.0, result[0])
	assert.Equal(t, 25.0, result[1])
	assert.Equal(t, 10.0, result[2])
	assert.Equal(t, 40.0, result[3])
	assert.Equal(t, 20.0, result[4])
}
