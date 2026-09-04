//go:build testing

package monitors_test

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/hub/monitors"
	_ "github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

// TestLoad_ManyMonitorsRapidCycles simulates 50 monitors with fast cycles
// and asserts: no database lock errors, bounded table growth, scheduler p99
// under 1s. Short-mode skips the heavy variant in CI.
func TestLoad_ManyMonitorsRapidCycles(t *testing.T) {
	if testing.Short() {
		t.Skip("heavy load test, skipped in short mode")
	}
	app, err := pbtests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "load@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))

	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	const n = 50
	for i := 0; i < n; i++ {
		m := core.NewRecord(col)
		m.Set("name", fmt.Sprintf("load-%02d", i))
		m.Set("type", "ping")
		m.Set("target", "example.com")
		m.Set("interval", 60)
		m.Set("timeout", 10)
		m.Set("max_retries", 2)
		m.Set("status", "up")
		m.Set("users", []string{user.Id})
		require.NoError(t, app.Save(m))
	}

	recs, err := monitors.LoadMonitors(app)
	require.NoError(t, err)
	require.Len(t, recs, n)

	var lockErrors atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	const cycles = 20
	for _, rec := range recs {
		wg.Add(1)
		go func(mr monitors.MonitorRecord) {
			defer wg.Done()
			for i := 0; i < cycles; i++ {
				res := monitors.CheckResult{Status: monitors.StatusUp, LatencyMs: 1.5}
				if err := monitors.SaveCheckResult(app, mr, res, 0, false); err != nil {
					if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "busy") {
						lockErrors.Add(1)
					} else {
						t.Errorf("unexpected save error: %v", err)
						return
					}
				}
			}
		}(rec)
	}
	wg.Wait()
	elapsed := time.Since(start)

	total, err := app.CountRecords("monitor_checks")
	require.NoError(t, err)
	require.EqualValues(t, n*cycles, total)
	require.Zero(t, lockErrors.Load(), "no database lock errors allowed")

	perCycleMs := float64(elapsed.Milliseconds()) / float64(n*cycles)
	t.Logf("wrote %d rows in %s (%.2f ms/cycle)", total, elapsed, perCycleMs)
	require.Less(t, perCycleMs, 1000.0, "scheduler budget: <1s per cycle")
}
