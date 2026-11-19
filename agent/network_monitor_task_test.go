//go:build testing

package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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
