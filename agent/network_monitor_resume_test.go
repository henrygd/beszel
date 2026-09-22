//go:build testing

package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func simulateMonitorSleep(g *monitorResumeGuard) {
	g.mu.Lock()
	g.lastTick = time.Now().Add(-time.Hour).Round(0)
	g.mu.Unlock()
}

func TestMonitorResumePause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g monitorResumeGuard
		g.start()
		defer g.shutdown()
		generation, allowed := g.snapshot()
		require.True(t, allowed)
		// Heartbeats alone must keep the guard current between infrequent probes.
		time.Sleep(time.Minute)
		synctest.Wait()
		steadyGeneration, allowed := g.snapshot()
		require.True(t, allowed)
		require.Equal(t, generation, steadyGeneration)
		// The probe, rather than the heartbeat, must detect this gap.
		simulateMonitorSleep(&g)
		next, allowed := g.snapshot()
		assert.False(t, allowed)
		assert.NotEqual(t, generation, next)
		time.Sleep(9 * time.Second)
		_, allowed = g.snapshot()
		assert.False(t, allowed)
		time.Sleep(time.Second)
		end, allowed := g.snapshot()
		assert.True(t, allowed)
		assert.Equal(t, next, end)
	})
}

func TestMonitorResumeGuardLifecycle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pm := newMonitorManagerWithProbe(func(context.Context, monitor.Config) (int64, error) { return 1, nil })
		defer pm.Stop()
		assert.Nil(t, pm.resumeGuard.stop)
		pm.SyncMonitors([]monitor.Config{{ID: "a", Interval: 3600}, {ID: "b", Interval: 3600}})
		stop := pm.resumeGuard.stop
		require.NotNil(t, stop)
		pm.DeleteMonitor("a")
		assert.Equal(t, stop, pm.resumeGuard.stop)
		pm.DeleteMonitor("b")
		assert.Nil(t, pm.resumeGuard.stop)
		select {
		case <-stop:
		default:
			t.Fatal("heartbeat was not stopped")
		}
		time.Sleep(time.Hour)
		_, err := pm.UpsertMonitor(monitor.Config{ID: "c", Interval: 3600}, false)
		require.NoError(t, err)
		_, allowed := pm.resumeGuard.snapshot()
		assert.True(t, allowed, "idle time must not trigger a resume pause")
		pm.SyncMonitors(nil)
		assert.Nil(t, pm.resumeGuard.stop)
	})
}

func TestMonitorResumeDiscardsInflightProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g monitorResumeGuard
		g.start()
		defer g.shutdown()
		task := newMonitorTask(monitor.Config{ID: "test"})
		defer task.cancel()
		task.resumeGuard = &g
		result := task.runProbe(func(context.Context, monitor.Config) (int64, error) {
			simulateMonitorSleep(&g)
			return 0, errors.New("network not ready")
		})
		assert.Nil(t, result)
		assert.Empty(t, task.history.samples)
		// Explicit requests may still run during the pause and record real failures.
		result = task.runProbe(func(context.Context, monitor.Config) (int64, error) {
			return 0, errors.New("unreachable")
		})
		require.NotNil(t, result)
		assert.Equal(t, 100.0, result.PacketLoss)
	})
}

func TestMonitorResumeSkipsScheduledProbes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		pm := newMonitorManagerWithProbe(func(context.Context, monitor.Config) (int64, error) {
			calls.Add(1)
			return 1, nil
		})
		defer pm.Stop()
		pm.SyncMonitors([]monitor.Config{{ID: "test", Interval: 1}})
		simulateMonitorSleep(&pm.resumeGuard)
		pm.resumeGuard.snapshot()
		time.Sleep(9 * time.Second)
		synctest.Wait()
		assert.Zero(t, calls.Load())
		assert.Empty(t, pm.GetResults(1000))
		time.Sleep(2 * time.Second)
		synctest.Wait()
		assert.Positive(t, calls.Load())
	})
}
