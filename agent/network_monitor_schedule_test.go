//go:build testing

package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorScheduleTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var calls atomic.Int32
		go runMonitorSchedule(ctx, 10*time.Second, 5*time.Second, func() { calls.Add(1) })
		synctest.Wait()
		time.Sleep(4 * time.Second)
		synctest.Wait()
		assert.Equal(t, 0, int(calls.Load()))
		time.Sleep(time.Second)
		synctest.Wait()
		assert.Equal(t, 1, int(calls.Load()))
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.Equal(t, 2, int(calls.Load()))
		cancel()
		synctest.Wait()
		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Equal(t, 2, int(calls.Load()))
	})
}

func TestMonitorScheduleSlowProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var calls atomic.Int32
		release := make(chan struct{})
		go runMonitorSchedule(ctx, time.Second, 0, func() {
			calls.Add(1)
			select {
			case <-release:
			case <-ctx.Done():
			}
		})
		synctest.Wait()
		assert.Equal(t, 1, int(calls.Load()))
		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Equal(t, 1, int(calls.Load()), "a slow probe must not spawn overlapping checks")
		close(release)
		synctest.Wait()
		assert.Equal(t, 1, int(calls.Load()), "missed intervals must not accumulate a backlog")
		time.Sleep(time.Second)
		synctest.Wait()
		assert.Equal(t, 2, int(calls.Load()))
		cancel()
		synctest.Wait()
	})
}

func TestMonitorScheduledAndImmediateRequestsShareProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		release := make(chan struct{})
		cfg := monitor.Config{ID: "test", Interval: 10}
		pm := newMonitorManagerWithProbe(func(ctx context.Context, config monitor.Config) (int64, error) {
			assert.Equal(t, cfg, config)
			calls.Add(1)
			<-release
			return 42, nil
		})
		defer pm.Stop()
		task := newMonitorTask(cfg)
		pm.monitors[cfg.ID] = task
		go runMonitorSchedule(task.ctx, 10*time.Second, 0, func() { task.runProbe(pm.probe) })
		synctest.Wait()
		results := make(chan *monitor.Result, 2)
		for range 2 {
			go func() {
				result, _ := pm.UpsertMonitor(cfg, true)
				results <- result
			}()
		}
		synctest.Wait()
		assert.Equal(t, 1, int(calls.Load()))
		assert.Empty(t, pm.GetResults(1000), "reading history must not wait for network I/O")
		close(release)
		synctest.Wait()
		first, second := <-results, <-results
		require.NotNil(t, first)
		require.NotNil(t, second)
		assert.Equal(t, int64(42), first.AvgResponse)
		assert.Equal(t, first, second)
		assert.NotSame(t, first, second, "callers must not share mutable result pointers")
		assert.Len(t, task.history.samples, 1)
		// A later explicit request must still perform a fresh probe.
		_, err := pm.UpsertMonitor(cfg, true)
		require.NoError(t, err)
		assert.Equal(t, 2, int(calls.Load()))
		assert.Len(t, task.history.samples, 2)
	})
}

func TestMonitorReplacementCancelsSharedProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := monitor.Config{ID: "test", Interval: 10}
		pm := newMonitorManagerWithProbe(func(ctx context.Context, config monitor.Config) (int64, error) {
			if config.Interval == 10 {
				<-ctx.Done()
				return 0, ctx.Err()
			}
			return 30, nil
		})
		defer pm.Stop()
		task := newMonitorTask(cfg)
		task.history.record(monitorSample{responseUs: 10, timestamp: time.Now()})
		pm.monitors[cfg.ID] = task
		results := make(chan *monitor.Result, 2)
		for range 2 {
			go func() {
				result, _ := pm.UpsertMonitor(cfg, true)
				results <- result
			}()
		}
		synctest.Wait()
		updated := cfg
		updated.Interval = 20
		result, err := pm.UpsertMonitor(updated, true)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, int64(20), result.AvgResponse)
		assert.Zero(t, result.PacketLoss)
		synctest.Wait()
		assert.Nil(t, <-results)
		assert.Nil(t, <-results)
		assert.Len(t, task.history.samples, 1)
		assert.Len(t, pm.monitors[cfg.ID].history.samples, 2)
	})
}

func TestMonitorInjectedProbeTimeoutRecordsLoss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pm := newMonitorManagerWithProbe(func(ctx context.Context, _ monitor.Config) (int64, error) {
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			<-ctx.Done()
			return 0, ctx.Err()
		})
		defer pm.Stop()
		start := time.Now()
		result, err := pm.UpsertMonitor(monitor.Config{ID: "test", Interval: 3600}, true)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 3*time.Second, time.Since(start))
		assert.Equal(t, 100.0, result.PacketLoss)
		assert.NoError(t, pm.monitors["test"].ctx.Err())
	})
}
