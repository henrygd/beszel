package alerts

import (
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// networkMonitorCache keeps just the enabled monitor IDs and probe intervals
// needed for the alert fast path. Names and targets are read only on transitions.
// Returned maps are immutable; configuration changes invalidate the whole entry.
type networkMonitorCache struct {
	app     core.App
	mu      sync.RWMutex
	systems map[string]map[string]int
}

func newNetworkMonitorCache(app core.App) *networkMonitorCache {
	c := &networkMonitorCache{app: app, systems: make(map[string]map[string]int)}
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
			old.GetInt("interval") != e.Record.GetInt("interval") {
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

func (c *networkMonitorCache) get(systemID string) (map[string]int, error) {
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
		ID       string `db:"id"`
		Interval int    `db:"interval"`
	}
	if err := c.app.DB().Select("id", "interval").From("network_monitors").
		Where(dbx.HashExp{"system": systemID, "enabled": true}).All(&rows); err != nil {
		return nil, err
	}
	monitors = make(map[string]int, len(rows))
	for _, row := range rows {
		monitors[row.ID] = row.Interval
	}
	c.systems[systemID] = monitors
	return monitors, nil
}
