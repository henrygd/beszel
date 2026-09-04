package monitors

import (
	"context"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// MonitorRecord maps the monitors collection fields used by the scheduler.
type MonitorRecord struct {
	ID                 string
	Name               string
	Type               MonitorType
	Target             string
	IntervalSeconds    int
	TimeoutSeconds     int
	MaxRetries         int
	UpsideDown         bool
	Paused             bool
	Config             map[string]any
	ConsecutiveFailure int
}

// ToMonitor converts a record to a scheduler Monitor.
func (r MonitorRecord) ToMonitor() Monitor {
	return Monitor{
		Name: r.Name, Type: r.Type, Target: r.Target,
		IntervalSeconds: r.IntervalSeconds, TimeoutSeconds: r.TimeoutSeconds,
		MaxRetries: r.MaxRetries, UpsideDown: r.UpsideDown, Config: r.Config,
	}
}

// RecordStore persists check results. It is implemented by the PocketBase
// hub (see LoadMonitors/SaveCheckResult) and faked in tests.
type RecordStore interface {
	LoadMonitors(ctx context.Context) ([]MonitorRecord, error)
	SaveCheckResult(ctx context.Context, rec MonitorRecord, res CheckResult, transition bool) error
}

// LoadMonitors reads all non-paused monitors from the monitors collection.
func LoadMonitors(app core.App) ([]MonitorRecord, error) {
	records, err := app.FindAllRecords("monitors")
	if err != nil {
		return nil, err
	}
	out := make([]MonitorRecord, 0, len(records))
	for _, rec := range records {
		if rec.GetBool("paused") {
			continue
		}
		out = append(out, recordToMonitor(rec))
	}
	return out, nil
}

func recordToMonitor(rec *core.Record) MonitorRecord {
	m := MonitorRecord{
		ID: rec.Id, Name: rec.GetString("name"),
		Type: MonitorType(rec.GetString("type")), Target: rec.GetString("target"),
		UpsideDown: rec.GetBool("upside_down"), Paused: rec.GetBool("paused"),
		ConsecutiveFailure: int(rec.GetFloat("consecutive_failures")),
	}
	m.IntervalSeconds = int(rec.GetFloat("interval"))
	if m.IntervalSeconds <= 0 {
		m.IntervalSeconds = 60
	}
	m.TimeoutSeconds = int(rec.GetFloat("timeout"))
	if m.TimeoutSeconds <= 0 {
		m.TimeoutSeconds = 10
	}
	m.MaxRetries = int(rec.GetFloat("max_retries"))
	m.Config = map[string]any{}
	_ = rec.UnmarshalJSONField("config", &m.Config)
	return m
}

// SaveCheckResult writes one check cycle in a single transaction: the
// monitor_checks history row plus the monitors status update. One
// transaction per cycle bounds SQLite single-writer pressure.
func SaveCheckResult(app core.App, rec MonitorRecord, res CheckResult, transition bool) error {
	_ = transition
	return app.RunInTransaction(func(txApp core.App) error {
		checksCol, err := txApp.FindCachedCollectionByNameOrId("monitor_checks")
		if err != nil {
			return err
		}
		check := core.NewRecord(checksCol)
		check.Set("monitor", rec.ID)
		check.Set("status", res.Status)
		check.Set("latency_ms", res.LatencyMs)
		if res.Code != nil {
			check.Set("code", *res.Code)
		}
		check.Set("message", res.Message)
		if len(res.Details) > 0 {
			check.Set("details", res.Details)
		}
		if res.CertDays != nil {
			check.Set("cert_days", *res.CertDays)
		}
		if err := txApp.SaveNoValidate(check); err != nil {
			return err
		}

		mon, err := txApp.FindRecordById("monitors", rec.ID)
		if err != nil {
			return err
		}
		mon.Set("status", res.Status)
		mon.Set("last_check", time.Now().UTC())
		mon.Set("last_latency_ms", res.LatencyMs)
		if res.CertDays != nil {
			mon.Set("cert_days", *res.CertDays)
		}
		failures := rec.ConsecutiveFailure
		if res.Status == StatusUp {
			failures = 0
		} else if res.Status == StatusDown {
			failures++
		}
		mon.Set("consecutive_failures", failures)
		return txApp.SaveNoValidate(mon)
	})
}

// Uptime24h computes the success ratio over the last 24h from history.
func Uptime24h(app core.App, monitorID string) (float64, error) {
	var rows []struct {
		Status string `db:"status"`
		Count  int    `db:"cnt"`
	}
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	err := app.DB().NewQuery("SELECT status, COUNT(*) as cnt FROM monitor_checks WHERE monitor = {:mon} AND created >= {:cutoff} GROUP BY status").
		Bind(dbx.Params{"mon": monitorID, "cutoff": cutoff}).All(&rows)
	if err != nil {
		return 0, err
	}
	var up, total int
	for _, r := range rows {
		total += r.Count
		if r.Status == StatusUp {
			up += r.Count
		}
	}
	if total == 0 {
		return 0, nil
	}
	return float64(up) / float64(total) * 100, nil
}
