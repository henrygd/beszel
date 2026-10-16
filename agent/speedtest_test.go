//go:build testing

package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/speedtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpeedtestManagerRunNowIsAsync(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		sm := newSpeedtestManagerWithRunner(func(ctx context.Context, config speedtest.Config) (speedtest.Result, error) {
			<-release
			return speedtest.Result{Download: 100, ServerID: config.ServerID}, nil
		})
		defer sm.Stop()

		require.NoError(t, sm.UpsertSpeedtest(speedtest.Config{ID: "a", ServerID: 7, Interval: 60}, true))
		synctest.Wait()
		assert.Nil(t, sm.GetResults(), "upsert must return before the run finishes")

		close(release)
		synctest.Wait()
		results := sm.GetResults()
		require.Contains(t, results, "a")
		assert.Equal(t, uint64(100), results["a"].Download)
		assert.Equal(t, uint32(7), results["a"].ServerID)
		assert.NotZero(t, results["a"].RunAt)
	})
}

func TestSpeedtestManagerSerializesRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var active, maxActive atomic.Int32
		sm := newSpeedtestManagerWithRunner(func(ctx context.Context, config speedtest.Config) (speedtest.Result, error) {
			n := active.Add(1)
			defer active.Add(-1)
			for {
				current := maxActive.Load()
				if n <= current || maxActive.CompareAndSwap(current, n) {
					break
				}
			}
			time.Sleep(30 * time.Second)
			return speedtest.Result{Download: uint64(config.ServerID)}, nil
		})
		defer sm.Stop()

		for i, id := range []string{"a", "b", "c"} {
			require.NoError(t, sm.UpsertSpeedtest(speedtest.Config{ID: id, ServerID: uint32(i + 1), Interval: 60}, true))
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		assert.Len(t, sm.GetResults(), 3)
		assert.Equal(t, int32(1), maxActive.Load())
	})
}

func TestSpeedtestManagerRecordsErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sm := newSpeedtestManagerWithRunner(func(context.Context, speedtest.Config) (speedtest.Result, error) {
			return speedtest.Result{}, errors.New("not installed")
		})
		defer sm.Stop()
		require.NoError(t, sm.UpsertSpeedtest(speedtest.Config{ID: "a", ServerID: 3, Interval: 60}, true))
		synctest.Wait()
		result := sm.GetResults()["a"]
		assert.Equal(t, "not installed", result.Error)
		assert.Equal(t, uint32(3), result.ServerID)
		assert.NotZero(t, result.RunAt)
	})
}

func TestSpeedtestManagerInterfaceChangeReplacesTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var interfaces []string
		sm := newSpeedtestManagerWithRunner(func(ctx context.Context, config speedtest.Config) (speedtest.Result, error) {
			mu.Lock()
			interfaces = append(interfaces, config.Interface)
			mu.Unlock()
			return speedtest.Result{}, nil
		})
		defer sm.Stop()

		require.NoError(t, sm.UpsertSpeedtest(speedtest.Config{ID: "a", Interval: 60}, true))
		synctest.Wait()
		previous := sm.tasks["a"]
		require.NoError(t, sm.UpsertSpeedtest(speedtest.Config{ID: "a", Interval: 60, Interface: "eth1"}, true))
		synctest.Wait()
		assert.NotSame(t, previous, sm.tasks["a"], "an interface change must replace the task")
		mu.Lock()
		assert.Equal(t, []string{"", "eth1"}, interfaces)
		mu.Unlock()
	})
}

func TestSpeedtestManagerSync(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		runs := map[uint32]int{}
		sm := newSpeedtestManagerWithRunner(func(ctx context.Context, config speedtest.Config) (speedtest.Result, error) {
			mu.Lock()
			runs[config.ServerID]++
			mu.Unlock()
			return speedtest.Result{ServerID: config.ServerID}, nil
		})
		defer sm.Stop()

		sm.SyncSpeedtests([]speedtest.Config{{ID: "a", ServerID: 1, Interval: 15}, {ID: "b", ServerID: 2, Interval: 15}, {ID: ""}})
		assert.Len(t, sm.tasks, 2)
		// First runs land on each speedtest's slot within one interval.
		time.Sleep(15*time.Minute + time.Second)
		synctest.Wait()
		assert.Len(t, sm.GetResults(), 2)

		// Changing the interval keeps the latest result; removed speedtests stop.
		sm.SyncSpeedtests([]speedtest.Config{{ID: "a", ServerID: 1, Interval: 30}})
		assert.Len(t, sm.tasks, 1)
		assert.Contains(t, sm.GetResults(), "a")

		require.NoError(t, sm.HandleSyncRequest(speedtest.SyncRequest{Action: monitor.SyncActionDelete, Config: speedtest.Config{ID: "a"}}))
		assert.Empty(t, sm.tasks)
		assert.Error(t, sm.HandleSyncRequest(speedtest.SyncRequest{Action: monitor.SyncActionDelete}))
		assert.Error(t, sm.HandleSyncRequest(speedtest.SyncRequest{Action: 99}))

		mu.Lock()
		before := runs[2]
		mu.Unlock()
		time.Sleep(time.Hour)
		synctest.Wait()
		mu.Lock()
		assert.Equal(t, before, runs[2], "removed speedtest must not run again")
		mu.Unlock()
	})
}

func TestSpeedtestManagerSuspend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int32
		var canceled atomic.Bool
		release := make(chan struct{})
		sm := newSpeedtestManagerWithRunner(func(ctx context.Context, config speedtest.Config) (speedtest.Result, error) {
			if runs.Add(1) == 2 {
				// The second run is in progress when the hub disconnects.
				select {
				case <-ctx.Done():
					canceled.Store(true)
					return speedtest.Result{}, ctx.Err()
				case <-release:
				}
			}
			return speedtest.Result{Download: 100}, nil
		})
		defer sm.Stop()

		cfg := speedtest.Config{ID: "a", Interval: 15}
		require.NoError(t, sm.UpsertSpeedtest(cfg, true))
		synctest.Wait()
		require.Equal(t, int32(1), runs.Load())
		require.NoError(t, sm.UpsertSpeedtest(cfg, true))
		synctest.Wait()
		require.Equal(t, int32(2), runs.Load())

		sm.Suspend()
		synctest.Wait()
		assert.True(t, canceled.Load(), "suspend must cancel the run in progress")
		assert.Contains(t, sm.GetResults(), "a", "suspend keeps results the hub hasn't collected yet")

		time.Sleep(time.Hour)
		synctest.Wait()
		assert.Equal(t, int32(2), runs.Load(), "suspended speedtests must not run")

		// The full sync on reconnect reschedules the unchanged config.
		sm.SyncSpeedtests([]speedtest.Config{cfg})
		time.Sleep(15 * time.Minute)
		synctest.Wait()
		assert.Equal(t, int32(3), runs.Load())
	})
}

func TestSpeedtestManagerSlotSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var runTimes []time.Time
		sm := newSpeedtestManagerWithRunner(func(context.Context, speedtest.Config) (speedtest.Result, error) {
			mu.Lock()
			runTimes = append(runTimes, time.Now())
			mu.Unlock()
			return speedtest.Result{}, nil
		})
		defer sm.Stop()

		// Interval 0 is raised to the minimum.
		interval := time.Duration(speedtest.MinInterval) * time.Minute
		offset := speedtestSlotOffset("a", interval)
		start := time.Now()
		require.NoError(t, sm.UpsertSpeedtest(speedtest.Config{ID: "a", Interval: 0}, true))
		time.Sleep(10 * interval)
		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()
		require.GreaterOrEqual(t, len(runTimes), 10)
		assert.Equal(t, start, runTimes[0], "runNow runs immediately")
		assert.GreaterOrEqual(t, runTimes[1].Sub(start), interval/2, "no scheduled run right after runNow")
		for i, runAt := range runTimes[1:] {
			assert.Zero(t, (time.Duration(runAt.UnixNano())-offset)%interval, "run %d is off its slot", i+1)
			if i > 0 {
				assert.Equal(t, interval, runAt.Sub(runTimes[i]))
			}
		}
	})
}

func TestSpeedtestManagerSlotSurvivesRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runAt := make(chan time.Time, 10)
		runner := func(context.Context, speedtest.Config) (speedtest.Result, error) {
			runAt <- time.Now()
			return speedtest.Result{}, nil
		}
		cfg := speedtest.Config{ID: "a", Interval: 30}

		sm := newSpeedtestManagerWithRunner(runner)
		sm.SyncSpeedtests([]speedtest.Config{cfg})
		first := <-runAt
		sm.Stop()

		// Restart partway through the interval: the next run keeps the same slot.
		time.Sleep(7 * time.Minute)
		sm = newSpeedtestManagerWithRunner(runner)
		defer sm.Stop()
		sm.SyncSpeedtests([]speedtest.Config{cfg})
		assert.Equal(t, 30*time.Minute, (<-runAt).Sub(first))
	})
}

func TestSpeedtestSlotOffset(t *testing.T) {
	interval := 15 * time.Minute
	offsets := map[time.Duration]bool{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		offset := speedtestSlotOffset(id, interval)
		assert.Equal(t, offset, speedtestSlotOffset(id, interval), "offset must be stable")
		assert.GreaterOrEqual(t, offset, time.Duration(0))
		assert.Less(t, offset, interval)
		assert.Zero(t, offset%time.Second)
		offsets[offset] = true
	}
	assert.Greater(t, len(offsets), 1, "different IDs should spread across the interval")
}

func TestNextSpeedtestSlot(t *testing.T) {
	interval := 15 * time.Minute
	offset := 3*time.Minute + 27*time.Second
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	assert.Equal(t, base.Add(offset), nextSpeedtestSlot(base, interval, offset).UTC())
	// A time exactly on a slot yields the following slot.
	assert.Equal(t, base.Add(offset+interval), nextSpeedtestSlot(base.Add(offset), interval, offset).UTC())
	assert.Equal(t, base.Add(offset+interval), nextSpeedtestSlot(base.Add(offset+time.Second), interval, offset).UTC())
}
