package agent

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorTaskAggregateLockedUsesRawSamplesForShortWindows(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	task := &monitorTask{}

	task.addSampleLocked(monitorSample{responseUs: 10, timestamp: now.Add(-90 * time.Second)})
	task.addSampleLocked(monitorSample{responseUs: 20, timestamp: now.Add(-30 * time.Second)})
	task.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-10 * time.Second)})

	agg := task.aggregateLocked(time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(2), agg.totalCount)
	assert.Equal(t, int64(1), agg.successCount)
	result := agg.result()
	assert.Equal(t, int64(20), result.AvgResponse)
	assert.Equal(t, int64(20), result.MinResponse)
	assert.Equal(t, int64(20), result.MaxResponse)
	assert.Equal(t, 50.0, result.PacketLoss)
}

func TestMonitorTaskAggregateLockedUsesMinuteBucketsForLongWindows(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 30, 0, time.UTC)
	task := &monitorTask{}

	task.addSampleLocked(monitorSample{responseUs: 10, timestamp: now.Add(-11 * time.Minute)})
	task.addSampleLocked(monitorSample{responseUs: 20, timestamp: now.Add(-9 * time.Minute)})
	task.addSampleLocked(monitorSample{responseUs: 40, timestamp: now.Add(-5 * time.Minute)})
	task.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-90 * time.Second)})
	task.addSampleLocked(monitorSample{responseUs: 30, timestamp: now.Add(-30 * time.Second)})

	agg := task.aggregateLocked(10*time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(4), agg.totalCount)
	assert.Equal(t, int64(3), agg.successCount)
	result := agg.result()
	assert.Equal(t, int64(30), result.AvgResponse)
	assert.Equal(t, int64(20), result.MinResponse)
	assert.Equal(t, int64(40), result.MaxResponse)
	assert.Equal(t, 25.0, result.PacketLoss)
}

func TestMonitorTaskAddSampleLockedTrimsRawSamplesButKeepsBucketHistory(t *testing.T) {
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	task := &monitorTask{}

	task.addSampleLocked(monitorSample{responseUs: 10, timestamp: now.Add(-10 * time.Minute)})
	task.addSampleLocked(monitorSample{responseUs: 20, timestamp: now})

	require.Len(t, task.samples, 1)
	assert.Equal(t, int64(20), task.samples[0].responseUs)

	agg := task.aggregateLocked(10*time.Minute, now)
	require.True(t, agg.hasData())
	assert.Equal(t, int64(2), agg.totalCount)
	assert.Equal(t, int64(2), agg.successCount)
	result := agg.result()
	assert.Equal(t, int64(15), result.AvgResponse)
	assert.Equal(t, int64(10), result.MinResponse)
	assert.Equal(t, int64(20), result.MaxResponse)
	assert.Equal(t, 0.0, result.PacketLoss)
}

func TestMonitorManagerGetResultsIncludesHourResponseRange(t *testing.T) {
	now := time.Now().UTC()
	task := &monitorTask{config: monitor.Config{ID: "monitor-1"}}
	task.addSampleLocked(monitorSample{responseUs: 10, timestamp: now.Add(-30 * time.Minute)})
	task.addSampleLocked(monitorSample{responseUs: 20, timestamp: now.Add(-9 * time.Minute)})
	task.addSampleLocked(monitorSample{responseUs: 40, timestamp: now.Add(-5 * time.Minute)})
	task.addSampleLocked(monitorSample{responseUs: 30, timestamp: now.Add(-50 * time.Second)})
	task.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-30 * time.Second)})

	pm := &MonitorManager{monitors: map[string]*monitorTask{"icmp:example.com": task}}

	results := pm.GetResults(uint16(time.Minute / time.Millisecond))
	result, ok := results["monitor-1"]
	require.True(t, ok)
	assert.Equal(t, int64(30), result.AvgResponse)
	assert.Equal(t, int64(25), result.AvgResponse1h)
	assert.Equal(t, int64(30), result.MinResponse)
	assert.Equal(t, int64(10), result.MinResponse1h)
	assert.Equal(t, int64(30), result.MaxResponse)
	assert.Equal(t, int64(40), result.MaxResponse1h)
	assert.Equal(t, 50.0, result.PacketLoss)
	assert.Equal(t, 20.0, result.PacketLoss1h)
}

func TestMonitorManagerGetResultsIncludesLossOnlyHourData(t *testing.T) {
	now := time.Now().UTC()
	task := &monitorTask{config: monitor.Config{ID: "monitor-1"}}
	task.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-30 * time.Second)})
	task.addSampleLocked(monitorSample{responseUs: -1, timestamp: now.Add(-10 * time.Second)})

	pm := &MonitorManager{monitors: map[string]*monitorTask{"icmp:example.com": task}}

	results := pm.GetResults(uint16(time.Minute / time.Millisecond))
	result, ok := results["monitor-1"]
	require.True(t, ok)
	assert.Equal(t, int64(0), result.AvgResponse)
	assert.Equal(t, int64(0), result.AvgResponse1h)
	assert.Equal(t, int64(0), result.MinResponse)
	assert.Equal(t, int64(0), result.MinResponse1h)
	assert.Equal(t, int64(0), result.MaxResponse)
	assert.Equal(t, int64(0), result.MaxResponse1h)
	assert.Equal(t, 100.0, result.PacketLoss)
	assert.Equal(t, 100.0, result.PacketLoss1h)
}

func TestMonitorConfigResultKeyUsesSyncedID(t *testing.T) {
	cfg := monitor.Config{ID: "monitor-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	assert.Equal(t, "monitor-1", cfg.ID)
}

func TestMonitorManagerSyncMonitorsSkipsConfigsWithoutStableID(t *testing.T) {
	validCfg := monitor.Config{ID: "monitor-1", Target: "ignored", Protocol: "noop", Interval: 10}
	invalidCfg := monitor.Config{Target: "ignored", Protocol: "noop", Interval: 10}

	pm := newMonitorManager()
	pm.SyncMonitors([]monitor.Config{validCfg, invalidCfg})
	defer pm.Stop()

	_, validExists := pm.monitors[validCfg.ID]
	_, invalidExists := pm.monitors[invalidCfg.ID]
	assert.True(t, validExists)
	assert.False(t, invalidExists)
}

func TestMonitorManagerSyncMonitorsStopsRemovedTasksButKeepsExisting(t *testing.T) {
	keepCfg := monitor.Config{ID: "monitor-1", Target: "ignored", Protocol: "noop", Interval: 10}
	removeCfg := monitor.Config{ID: "monitor-2", Target: "ignored", Protocol: "noop", Interval: 10}

	keptTask := &monitorTask{config: keepCfg, cancel: make(chan struct{})}
	removedTask := &monitorTask{config: removeCfg, cancel: make(chan struct{})}
	pm := &MonitorManager{
		monitors: map[string]*monitorTask{
			keepCfg.ID:   keptTask,
			removeCfg.ID: removedTask,
		},
	}

	pm.SyncMonitors([]monitor.Config{keepCfg})

	assert.Same(t, keptTask, pm.monitors[keepCfg.ID])
	_, exists := pm.monitors[removeCfg.ID]
	assert.False(t, exists)

	select {
	case <-removedTask.cancel:
	default:
		t.Fatal("expected removed monitor task to be cancelled")
	}

	select {
	case <-keptTask.cancel:
		t.Fatal("expected existing monitor task to remain active")
	default:
	}
}

func TestMonitorManagerSyncMonitorsRestartsChangedConfig(t *testing.T) {
	originalCfg := monitor.Config{ID: "monitor-1", Target: "ignored-a", Protocol: "noop", Interval: 10}
	updatedCfg := monitor.Config{ID: "monitor-1", Target: "ignored-b", Protocol: "noop", Interval: 10}
	originalTask := &monitorTask{config: originalCfg, cancel: make(chan struct{})}
	pm := &MonitorManager{
		monitors: map[string]*monitorTask{
			originalCfg.ID: originalTask,
		},
	}

	pm.SyncMonitors([]monitor.Config{updatedCfg})
	defer pm.Stop()

	restartedTask := pm.monitors[updatedCfg.ID]
	assert.NotSame(t, originalTask, restartedTask)
	assert.Equal(t, updatedCfg, restartedTask.config)

	select {
	case <-originalTask.cancel:
	default:
		t.Fatal("expected changed monitor task to be cancelled")
	}
}

func TestMonitorManagerApplySyncUpsertRunsImmediatelyAndReturnsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	pm := &MonitorManager{
		monitors:   make(map[string]*monitorTask),
		httpClient: server.Client(),
	}

	resp, err := pm.HandleSyncRequest(monitor.SyncRequest{
		Action: monitor.SyncActionUpsert,
		Config: monitor.Config{ID: "monitor-1", Target: server.URL, Protocol: "http", Interval: 10},
		RunNow: true,
	})
	defer pm.Stop()

	require.NoError(t, err)
	assert.GreaterOrEqual(t, resp.Result.AvgResponse, int64(0))
	assert.Equal(t, 0.0, resp.Result.PacketLoss)
	assert.Equal(t, 0.0, resp.Result.PacketLoss1h)

	task := pm.monitors["monitor-1"]
	require.NotNil(t, task)
	task.mu.Lock()
	defer task.mu.Unlock()
	require.Len(t, task.samples, 1)
}

func TestMonitorManagerUpsertMonitorKeepsHistoryWhenOnlyIntervalChanges(t *testing.T) {
	originalCfg := monitor.Config{ID: "monitor-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	updatedCfg := monitor.Config{ID: "monitor-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 30}
	now := time.Now().UTC()

	existingTask := &monitorTask{config: originalCfg, cancel: make(chan struct{})}
	existingTask.addSampleLocked(monitorSample{responseUs: 12, timestamp: now.Add(-50 * time.Minute)})
	existingTask.addSampleLocked(monitorSample{responseUs: 24, timestamp: now.Add(-30 * time.Second)})

	pm := &MonitorManager{
		monitors: map[string]*monitorTask{originalCfg.ID: existingTask},
	}

	result, err := pm.UpsertMonitor(updatedCfg, false)
	defer pm.Stop()

	require.NoError(t, err)
	assert.Nil(t, result)

	updatedTask := pm.monitors[updatedCfg.ID]
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
	assert.Equal(t, int64(18), agg.avgResponse())

	select {
	case <-existingTask.cancel:
	default:
		t.Fatal("expected original monitor task to be cancelled")
	}
}

func TestMonitorManagerApplySyncDeleteRemovesTask(t *testing.T) {
	config := monitor.Config{ID: "monitor-1", Target: "1.1.1.1", Protocol: "icmp", Interval: 10}
	task := &monitorTask{config: config, cancel: make(chan struct{})}
	pm := &MonitorManager{
		monitors: map[string]*monitorTask{config.ID: task},
	}

	_, err := pm.HandleSyncRequest(monitor.SyncRequest{
		Action: monitor.SyncActionDelete,
		Config: monitor.Config{ID: config.ID},
	})

	require.NoError(t, err)
	_, exists := pm.monitors[config.ID]
	assert.False(t, exists)

	select {
	case <-task.cancel:
	default:
		t.Fatal("expected deleted monitor task to be cancelled")
	}
}

func TestMonitorManagerGetRandomDelay(t *testing.T) {
	for i := 1000; i < 360_000; i += 1000 {
		delay := getStagger(int64(i))
		assert.GreaterOrEqual(t, delay, time.Duration(i/2)*time.Millisecond)
		assert.LessOrEqual(t, delay, time.Duration(i)*time.Millisecond)
	}
}

func TestMonitorHTTP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		responseUs, err := monitorHTTP(server.Client(), server.URL)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, responseUs, int64(0))
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()

		responseUs, err := monitorHTTP(server.Client(), server.URL)
		assert.Equal(t, int64(-1), responseUs)
		require.Error(t, err)
	})
}

func TestMonitorTCP(t *testing.T) {
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
		responseUs, err := monitorTCP("127.0.0.1", port)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, responseUs, int64(0))
		<-accepted
	})

	t.Run("connection failure", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		port := uint16(listener.Addr().(*net.TCPAddr).Port)
		require.NoError(t, listener.Close())

		responseUs, err := monitorTCP("127.0.0.1", port)
		assert.Equal(t, int64(-1), responseUs)
		require.Error(t, err)
	})
}

func TestMonitorDNS(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		responseUs, err := monitorDNS("localhost")
		require.NoError(t, err)
		assert.GreaterOrEqual(t, responseUs, int64(0))
	})

	t.Run("lookup failure", func(t *testing.T) {
		responseUs, err := monitorDNS("")
		assert.Equal(t, int64(-1), responseUs)
		require.Error(t, err)
	})
}
