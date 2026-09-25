//go:build testing

package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorFailureLogCooldown(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	synctest.Test(t, func(t *testing.T) {
		task := newMonitorTask(monitor.Config{ID: "test", Target: "example.test", Protocol: "tcp"})
		defer task.cancel()
		failure := errors.New("connection refused")
		probe := func(context.Context, monitor.Config) (int64, error) { return 42, failure }
		var samples int64
		check := func(wantLog bool) {
			t.Helper()
			logs.Reset()
			result := task.runProbe(probe)
			require.NotNil(t, result)
			samples++
			assert.Equal(t, samples, result.SampleCount, "suppressed warnings must still record samples")
			if !wantLog {
				assert.Empty(t, logs.String())
			} else {
				assert.Contains(t, logs.String(), `msg="monitor failed"`)
				assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("\n")))
			}
		}

		check(true)
		check(false)
		time.Sleep(5*time.Minute - time.Nanosecond)
		check(false)
		time.Sleep(time.Nanosecond)
		check(true)
		check(false)
		time.Sleep(5 * time.Minute)
		check(true)
		check(false)

		// Recovery clears the cooldown.
		failure = nil
		check(false)
		failure = errors.New("connection refused again")
		check(true)

		// Another monitor has its own cooldown.
		other := newMonitorTask(task.config)
		defer other.cancel()
		logs.Reset()
		require.NotNil(t, other.runProbe(probe))
		assert.Contains(t, logs.String(), `msg="monitor failed"`)

		// A canceled probe must not publish a failure or emit a warning.
		logs.Reset()
		result := other.runProbe(func(context.Context, monitor.Config) (int64, error) {
			other.cancel()
			return -1, context.Canceled
		})
		assert.Nil(t, result)
		assert.Empty(t, logs.String())
	})
}

func TestMonitorICMPCountReflectsPartialLoss(t *testing.T) {
	var calls atomic.Int32
	// Of four pings: two reply (1ms, 3ms), two are lost.
	probe := func(context.Context, monitor.Config) (int64, error) {
		switch calls.Add(1) {
		case 1:
			return 1000, nil
		case 2:
			return 3000, nil
		default:
			return -1, errors.New("timeout")
		}
	}

	task := newMonitorTask(monitor.Config{ID: "test", Target: "example.test", Protocol: "icmp", Count: 4})
	defer task.cancel()
	result := task.runProbe(probe)
	require.NotNil(t, result)
	assert.EqualValues(t, 4, calls.Load())
	assert.EqualValues(t, 1, result.SampleCount, "a check is one sample regardless of count")
	assert.EqualValues(t, 4, result.TotalCount)
	assert.EqualValues(t, 2, result.SuccessCount)
	assert.EqualValues(t, 2000, result.AvgResponse)
	assert.EqualValues(t, 1000, result.MinResponse)
	assert.EqualValues(t, 3000, result.MaxResponse)
	assert.Equal(t, 50.0, result.PacketLoss)

	// Every ping lost still records loss for the whole check.
	task = newMonitorTask(monitor.Config{ID: "lost", Target: "example.test", Protocol: "icmp", Count: 3})
	defer task.cancel()
	result = task.runProbe(func(context.Context, monitor.Config) (int64, error) { return -1, errors.New("timeout") })
	require.NotNil(t, result)
	assert.EqualValues(t, 3, result.TotalCount)
	assert.Equal(t, 100.0, result.PacketLoss)

	// Count is ignored for other protocols and defaults to one ping.
	calls.Store(0)
	for _, cfg := range []monitor.Config{
		{ID: "a", Protocol: "icmp"},
		{ID: "b", Protocol: "tcp", Count: 5},
	} {
		task = newMonitorTask(cfg)
		defer task.cancel()
		task.runProbe(func(context.Context, monitor.Config) (int64, error) { calls.Add(1); return 1000, nil })
	}
	assert.EqualValues(t, 2, calls.Load())
}
