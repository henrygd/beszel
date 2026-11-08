package alerts

import (
	"encoding/json"
	"sync"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// cachedMonitor holds the monitor fields needed for the alert fast path.
type cachedMonitor struct {
	interval int
	// certExpires is the stored certificate expiry in Unix milliseconds, or zero if unknown.
	certExpires int64
}

// networkMonitorCache keeps just the enabled monitor IDs, probe intervals, and
// certificate expiry times needed for the alert fast path. Names and targets are
// read only on transitions. Returned maps are immutable; configuration and
// certificate changes invalidate the whole entry.
type networkMonitorCache struct {
	app     core.App
	mu      sync.RWMutex
	systems map[string]map[string]cachedMonitor
}

func newNetworkMonitorCache(app core.App) *networkMonitorCache {
	c := &networkMonitorCache{app: app, systems: make(map[string]map[string]cachedMonitor)}
	invalidate := func(e *core.RecordEvent) error {
		c.invalidate(e.Record.GetString("system"))
		return e.Next()
	}
	app.OnRecordAfterCreateSuccess("network_monitors").BindFunc(invalidate)
	app.OnRecordAfterDeleteSuccess("network_monitors").BindFunc(invalidate)
	app.OnRecordAfterUpdateSuccess("network_monitors").BindFunc(func(e *core.RecordEvent) error {
		old := e.Record.Original()
		// Realtime metric saves also invoke this hook. They must not evict config.
		if old.GetString("system") != e.Record.GetString("system") ||
			old.GetBool("enabled") != e.Record.GetBool("enabled") ||
			old.GetInt("interval") != e.Record.GetInt("interval") ||
			old.GetString("certInfo") != e.Record.GetString("certInfo") {
			c.invalidate(old.GetString("system"))
			c.invalidate(e.Record.GetString("system"))
		}
		return e.Next()
	})
	app.OnRecordAfterDeleteSuccess("systems").BindFunc(func(e *core.RecordEvent) error {
		c.invalidate(e.Record.Id)
		return e.Next()
	})
	return c
}

func (c *networkMonitorCache) invalidate(systemID string) {
	c.mu.Lock()
	delete(c.systems, systemID)
	c.mu.Unlock()
}

// certChanged reports whether a cached monitor's certificate expiry differs from
// expires. Systems that are not cached load fresh data when next needed.
func (c *networkMonitorCache) certChanged(systemID, monitorID string, expires int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.systems[systemID][monitorID]
	return ok && m.certExpires != expires
}

func (c *networkMonitorCache) get(systemID string) (map[string]cachedMonitor, error) {
	c.mu.RLock()
	monitors, ok := c.systems[systemID]
	c.mu.RUnlock()
	if ok {
		return monitors, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if monitors, ok := c.systems[systemID]; ok {
		return monitors, nil
	}
	// Keep the lock through the load so a concurrent config change cannot be
	// invalidated first and then overwritten by the older query result.
	var rows []struct {
		ID       string        `db:"id"`
		Interval int           `db:"interval"`
		CertInfo types.JSONRaw `db:"certInfo"`
	}
	if err := c.app.DB().Select("id", "interval", "certInfo").From("network_monitors").
		Where(dbx.HashExp{"system": systemID, "enabled": true}).All(&rows); err != nil {
		return nil, err
	}
	monitors = make(map[string]cachedMonitor, len(rows))
	for _, row := range rows {
		var cert monitor.CertInfo
		// Missing or malformed info leaves the expiry unknown.
		_ = json.Unmarshal(row.CertInfo, &cert)
		monitors[row.ID] = cachedMonitor{interval: row.Interval, certExpires: cert.Expires}
	}
	c.systems[systemID] = monitors
	return monitors, nil
}
