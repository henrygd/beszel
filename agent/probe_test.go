package agent

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/probe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeTaskAggregateLockedUsesRawSamplesForShortWindows(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	task := &probeTask{}

	task.addSampleLocked(probeSample{responseUs: 10, timestamp: now.Add(-90 * time.Second)})
	task.addSampleLocked(probeSample{responseUs: 20, timestamp: now.Add(-30 * time.Second)})
	task.addSampleLocked(probeSample{responseUs: -1, timestamp: now.Add(-10 * time.Second)})

	agg := task.aggregateLocked(time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(2), agg.totalCount)
	assert.Equal(t, int64(1), agg.successCount)
	assert.Equal(t, 20.0, agg.result()[0])
	assert.Equal(t, 20.0, agg.result()[1])
	assert.Equal(t, 20.0, agg.result()[2])
	assert.Equal(t, 50.0, agg.result()[3])
}

func TestProbeTaskAggregateLockedUsesMinuteBucketsForLongWindows(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 30, 0, time.UTC)
	task := &probeTask{}

	task.addSampleLocked(probeSample{responseUs: 10, timestamp: now.Add(-11 * time.Minute)})
	task.addSampleLocked(probeSample{responseUs: 20, timestamp: now.Add(-9 * time.Minute)})
	task.addSampleLocked(probeSample{responseUs: 40, timestamp: now.Add(-5 * time.Minute)})
	task.addSampleLocked(probeSample{responseUs: -1, timestamp: now.Add(-90 * time.Second)})
	task.addSampleLocked(probeSample{responseUs: 30, timestamp: now.Add(-30 * time.Second)})

	agg := task.aggregateLocked(10*time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(4), agg.totalCount)
	assert.Equal(t, int64(3), agg.successCount)
	assert.Equal(t, 30.0, agg.result()[0])
	assert.Equal(t, 20.0, agg.result()[1])
	assert.Equal(t, 40.0, agg.result()[2])
	assert.Equal(t, 25.0, agg.result()[3])
}

func TestProbeTaskAddSampleLockedTrimsRawSamplesButKeepsBucketHistory(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	task := &probeTask{}

	task.addSampleLocked(probeSample{responseUs: 10, timestamp: now.Add(-10 * time.Minute)})
	task.addSampleLocked(probeSample{responseUs: 20, timestamp: now})

	require.Len(t, task.samples, 1)
	assert.Equal(t, int64(20), task.samples[0].responseUs)

	agg := task.aggregateLocked(10*time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(2), agg.totalCount)
	assert.Equal(t, int64(2), agg.successCount)
	assert.Equal(t, 15.0, agg.result()[0])
	assert.Equal(t, 10.0, agg.result()[1])
	assert.Equal(t, 20.0, agg.result()[2])
	assert.Equal(t, 0.0, agg.result()[3])
}

func TestProbeManagerGetResultsIncludesHourResponseRange(t *testing.T) {
	now := time.Now().UTC()
	task := &probeTask{config: probe.Config{ID: "probe-1"}}
	task.addSampleLocked(probeSample{responseUs: 10, timestamp: now.Add(-30 * time.Minute)})
	task.addSampleLocked(probeSample{responseUs: 20, timestamp: now.Add(-9 * time.Minute)})
	task.addSampleLocked(probeSample{responseUs: 40, timestamp: now.Add(-5 * time.Minute)})
	task.addSampleLocked(probeSample{responseUs: 30, timestamp: now.Add(-50 * time.Second)})
	task.addSampleLocked(probeSample{responseUs: -1, timestamp: now.Add(-30 * time.Second)})

	pm := &ProbeManager{probes: map[string]*probeTask{"icmp:example.com": task}}

	results := pm.GetResults(uint16(time.Minute / time.Millisecond))
	result, ok := results["probe-1"]
	require.True(t, ok)
	require.Len(t, result, 6)
	assert.Equal(t, 30.0, result[0])
	assert.Equal(t, 25.0, result[1])
	assert.Equal(t, 10.0, result[2])
	assert.Equal(t, 40.0, result[3])
	assert.Equal(t, 50.0, result[4])
	assert.Equal(t, 20.0, result[5])
}

func TestProbeManagerGetResultsIncludesLossOnlyHourData(t *testing.T) {
	now := time.Now().UTC()
	task := &probeTask{config: probe.Config{ID: "probe-1"}}
	task.addSampleLocked(probeSample{responseUs: -1, timestamp: now.Add(-30 * time.Second)})
	task.addSampleLocked(probeSample{responseUs: -1, timestamp: now.Add(-10 * time.Second)})

	pm := &ProbeManager{probes: map[string]*probeTask{"icmp:example.com": task}}

	results := pm.GetResults(uint16(time.Minute / time.Millisecond))
	result, ok := results["probe-1"]
	require.True(t, ok)
	require.Len(t, result, 6)
	assert.Equal(t, 0.0, result[0])
	assert.Equal(t, 0.0, result[1])
	assert.Equal(t, 0.0, result[2])
	assert.Equal(t, 0.0, result[3])
	assert.Equal(t, 100.0, result[4])
	assert.Equal(t, 100.0, result[5])
}

func TestProbeConfigResultKeyUsesSyncedID(t *testing.T) {
	cfg := probe.Config{ID: "probe-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	assert.Equal(t, "probe-1", cfg.ID)
}

func TestProbeManagerSyncProbesSkipsConfigsWithoutStableID(t *testing.T) {
	validCfg := probe.Config{ID: "probe-1", Target: "ignored", Protocol: "noop", Interval: 10}
	invalidCfg := probe.Config{Target: "ignored", Protocol: "noop", Interval: 10}

	pm := newProbeManager()
	pm.SyncProbes([]probe.Config{validCfg, invalidCfg})
	defer pm.Stop()

	_, validExists := pm.probes[validCfg.ID]
	_, invalidExists := pm.probes[invalidCfg.ID]
	assert.True(t, validExists)
	assert.False(t, invalidExists)
}

func TestProbeManagerSyncProbesStopsRemovedTasksButKeepsExisting(t *testing.T) {
	keepCfg := probe.Config{ID: "probe-1", Target: "ignored", Protocol: "noop", Interval: 10}
	removeCfg := probe.Config{ID: "probe-2", Target: "ignored", Protocol: "noop", Interval: 10}

	keptTask := &probeTask{config: keepCfg, cancel: make(chan struct{})}
	removedTask := &probeTask{config: removeCfg, cancel: make(chan struct{})}
	pm := &ProbeManager{
		probes: map[string]*probeTask{
			keepCfg.ID:   keptTask,
			removeCfg.ID: removedTask,
		},
	}

	pm.SyncProbes([]probe.Config{keepCfg})

	assert.Same(t, keptTask, pm.probes[keepCfg.ID])
	_, exists := pm.probes[removeCfg.ID]
	assert.False(t, exists)

	select {
	case <-removedTask.cancel:
	default:
		t.Fatal("expected removed probe task to be cancelled")
	}

	select {
	case <-keptTask.cancel:
		t.Fatal("expected existing probe task to remain active")
	default:
	}
}

func TestProbeManagerSyncProbesRestartsChangedConfig(t *testing.T) {
	originalCfg := probe.Config{ID: "probe-1", Target: "ignored-a", Protocol: "noop", Interval: 10}
	updatedCfg := probe.Config{ID: "probe-1", Target: "ignored-b", Protocol: "noop", Interval: 10}
	originalTask := &probeTask{config: originalCfg, cancel: make(chan struct{})}
	pm := &ProbeManager{
		probes: map[string]*probeTask{
			originalCfg.ID: originalTask,
		},
	}

	pm.SyncProbes([]probe.Config{updatedCfg})
	defer pm.Stop()

	restartedTask := pm.probes[updatedCfg.ID]
	assert.NotSame(t, originalTask, restartedTask)
	assert.Equal(t, updatedCfg, restartedTask.config)

	select {
	case <-originalTask.cancel:
	default:
		t.Fatal("expected changed probe task to be cancelled")
	}
}

func TestProbeManagerApplySyncUpsertRunsImmediatelyAndReturnsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	pm := &ProbeManager{
		probes:     make(map[string]*probeTask),
		httpClient: server.Client(),
	}

	resp, err := pm.HandleSyncRequest(probe.SyncRequest{
		Action: probe.SyncActionUpsert,
		Config: probe.Config{ID: "probe-1", Target: server.URL, Protocol: "http", Interval: 10},
		RunNow: true,
	})
	defer pm.Stop()

	require.NoError(t, err)
	require.Len(t, resp.Result, 6)
	assert.GreaterOrEqual(t, resp.Result[0], 0.0)
	assert.Equal(t, 0.0, resp.Result[4])
	assert.Equal(t, 0.0, resp.Result[5])

	task := pm.probes["probe-1"]
	require.NotNil(t, task)
	task.mu.Lock()
	defer task.mu.Unlock()
	require.Len(t, task.samples, 1)
}

func TestProbeManagerUpsertProbeKeepsHistoryWhenOnlyIntervalChanges(t *testing.T) {
	originalCfg := probe.Config{ID: "probe-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	updatedCfg := probe.Config{ID: "probe-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 30}
	now := time.Now().UTC()

	existingTask := &probeTask{config: originalCfg, cancel: make(chan struct{})}
	existingTask.addSampleLocked(probeSample{responseUs: 12, timestamp: now.Add(-50 * time.Minute)})
	existingTask.addSampleLocked(probeSample{responseUs: 24, timestamp: now.Add(-30 * time.Second)})

	pm := &ProbeManager{
		probes: map[string]*probeTask{originalCfg.ID: existingTask},
	}

	result, err := pm.UpsertProbe(updatedCfg, false)
	defer pm.Stop()

	require.NoError(t, err)
	assert.Nil(t, result)

	updatedTask := pm.probes[updatedCfg.ID]
	require.NotNil(t, updatedTask)
	assert.NotSame(t, existingTask, updatedTask)
	assert.Equal(t, updatedCfg, updatedTask.config)

	updatedTask.mu.Lock()
	defer updatedTask.mu.Unlock()
	require.Len(t, updatedTask.samples, 1)
	assert.Equal(t, int64(24), updatedTask.samples[0].responseUs)

	agg := updatedTask.aggregateLocked(time.Hour, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(2), agg.totalCount)
	assert.Equal(t, int64(2), agg.successCount)
	assert.Equal(t, 18.0, agg.avgResponse())

	select {
	case <-existingTask.cancel:
	default:
		t.Fatal("expected original probe task to be cancelled")
	}
}

func TestProbeManagerApplySyncDeleteRemovesTask(t *testing.T) {
	config := probe.Config{ID: "probe-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	task := &probeTask{config: config, cancel: make(chan struct{})}
	pm := &ProbeManager{
		probes: map[string]*probeTask{config.ID: task},
	}

	_, err := pm.HandleSyncRequest(probe.SyncRequest{
		Action: probe.SyncActionDelete,
		Config: probe.Config{ID: config.ID},
	})

	require.NoError(t, err)
	_, exists := pm.probes[config.ID]
	assert.False(t, exists)

	select {
	case <-task.cancel:
	default:
		t.Fatal("expected deleted probe task to be cancelled")
	}
}

func TestProbeManagerGetRandomDelay(t *testing.T) {
	for i := 1000; i < 360_000; i += 1000 {
		delay := getStagger(int64(i))
		assert.GreaterOrEqual(t, delay, time.Duration(i/2)*time.Millisecond)
		assert.LessOrEqual(t, delay, time.Duration(i)*time.Millisecond)
	}
}

func TestProbeHTTP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		responseUs := probeHTTP(server.Client(), server.URL)
		assert.GreaterOrEqual(t, responseUs, int64(0))
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()

		assert.Equal(t, int64(-1), probeHTTP(server.Client(), server.URL))
	})
}

func TestProbeTCP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		accepted := make(chan struct{})
		go func() {
			defer close(accepted)
			conn, err := listener.Accept()
			if err == nil {
				_ = conn.Close()
			}
		}()

		port := uint16(listener.Addr().(*net.TCPAddr).Port)
		responseUs := probeTCP("127.0.0.1", port)
		assert.GreaterOrEqual(t, responseUs, int64(0))
		<-accepted
	})

	t.Run("connection failure", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		port := uint16(listener.Addr().(*net.TCPAddr).Port)
		require.NoError(t, listener.Close())

		assert.Equal(t, int64(-1), probeTCP("127.0.0.1", port))
	})
}
