package records

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// refreshMonitorUptime recomputes monitors.uptime_24h from monitor_checks.
// It runs inside the existing "create longer records" cron (every 10 min),
// so no new cron is needed. Checks from manual test runs are never written
// to monitor_checks, so they cannot skew the ratio.
func refreshMonitorUptime(app core.App) error {
	var ids []struct {
		ID string `db:"id"`
	}
	if err := app.DB().NewQuery("SELECT id FROM monitors").All(&ids); err != nil {
		return err
	}
	for _, m := range ids {
		var rows []struct {
			Status string `db:"status"`
			Count  int    `db:"cnt"`
		}
		if err := app.DB().NewQuery("SELECT status, COUNT(*) as cnt FROM monitor_checks WHERE monitor = {:mon} AND created >= datetime('now', '-1 day') GROUP BY status").
			Bind(dbx.Params{"mon": m.ID}).All(&rows); err != nil {
			return err
		}
		var up, total int
		for _, r := range rows {
			total += r.Count
			if r.Status == "up" || r.Status == "warn" {
				up += r.Count
			}
		}
		var ratio float64
		if total > 0 {
			ratio = float64(up) / float64(total) * 100
		}
		if _, err := app.DB().NewQuery("UPDATE monitors SET uptime_24h = {:ratio} WHERE id = {:id}").
			Bind(dbx.Params{"ratio": ratio, "id": m.ID}).Execute(); err != nil {
			return err
		}
	}
	return nil
}
