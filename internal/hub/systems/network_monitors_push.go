package systems

import (
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// pushGracePeriod is how long a heartbeat may be late before the monitor counts as down.
const pushGracePeriod = time.Minute

// pushMonitorResult evaluates one push monitor. A heartbeat is on time if the last one
// arrived within the monitor's interval plus the grace period.
//
// The result is binary, like an up/down status: loss is 0 or 100 so that loss alerts
// trigger as soon as a heartbeat is overdue and resolve with the next one. LastProbeAt
// is aligned to the minute, so repeated evaluations within the same minute produce a
// single stats record.
func pushMonitorResult(interval int, lastPushMs int64, now time.Time) monitor.Result {
	deadline := time.UnixMilli(lastPushMs).Add(time.Duration(interval)*time.Second + pushGracePeriod)
	result := monitor.Result{
		LastProbeAt: now.Truncate(time.Minute).UnixMilli(),
		// A push monitor is only evaluated after its first heartbeat, so no alert warm-up is needed.
		SampleCount: 3,
		TotalCount:  1,
	}
	if !now.After(deadline) {
		result.SuccessCount = 1
	} else {
		result.PacketLoss = 100
		result.PacketLoss1h = 100
	}
	return result
}

// pushMonitorResults evaluates the system's enabled push monitors that have received at
// least one heartbeat. Monitors that never received one produce no result, so they don't
// alert before they are set up.
func (sys *System) pushMonitorResults(app core.App, now time.Time) (map[string]monitor.Result, error) {
	var rows []struct {
		Id       string  `db:"id"`
		Interval int     `db:"interval"`
		LastPush float64 `db:"lastPush"`
	}
	err := app.DB().
		NewQuery("SELECT id, interval, lastPush FROM network_monitors WHERE system = {:system} AND protocol = 'push' AND enabled = true AND lastPush > 0").
		Bind(dbx.Params{"system": sys.Id}).
		All(&rows)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	results := make(map[string]monitor.Result, len(rows))
	for _, row := range rows {
		results[row.Id] = pushMonitorResult(row.Interval, int64(row.LastPush), now)
	}
	return results, nil
}
