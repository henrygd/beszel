package alerts

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/store"
)

// CachedAlertData represents the relevant fields of an alert record for status checking and updates.
type CachedAlertData struct {
	Id        string
	SystemID  string
	UserID    string
	Name      string
	Value     float64
	Triggered bool
	Min       uint8
	// Created   types.DateTime
}

func (a *CachedAlertData) PopulateFromRecord(record *core.Record) {
	a.Id = record.Id
	a.SystemID = record.GetString("system")
	a.UserID = record.GetString("user")
	a.Name = record.GetString("name")
	a.Value = record.GetFloat("value")
	a.Triggered = record.GetBool("triggered")
	a.Min = uint8(record.GetInt("min"))
	// a.Created = record.GetDateTime("created")
}

// AlertsCache provides an in-memory cache for system alerts.
type AlertsCache struct {
	app       core.App
	store     *store.Store[string, *store.Store[string, CachedAlertData]]
	populated bool
}

// NewAlertsCache creates a new instance of SystemAlertsCache.
func NewAlertsCache(app core.App) *AlertsCache {
	c := AlertsCache{
		app:   app,
		store: store.New(map[string]*store.Store[string, CachedAlertData]{}),
	}
	return c.bindEvents()
}

// bindEvents sets up event listeners to keep the cache in sync with database changes.
func (c *AlertsCache) bindEvents() *AlertsCache {
	c.app.OnRecordAfterUpdateSuccess("alerts").BindFunc(func(e *core.RecordEvent) error {
		// c.Delete(e.Record.Original()) // this would be needed if the system field on an existing alert was changed, however we don't currently allow that in the UI so we'll leave it commented out
		c.Update(e.Record)
		return e.Next()
	})
	c.app.OnRecordAfterDeleteSuccess("alerts").BindFunc(func(e *core.RecordEvent) error {
		c.Delete(e.Record)
		return e.Next()
	})
	c.app.OnRecordAfterCreateSuccess("alerts").BindFunc(func(e *core.RecordEvent) error {
		c.Update(e.Record)
		return e.Next()
	})
	return c
}

// PopulateFromDB clears current entries and loads all alerts from the database into the cache.
func (c *AlertsCache) PopulateFromDB(force bool) error {
	if !force && c.populated {
		return nil
	}
	records, err := c.app.FindAllRecords("alerts")
	if err != nil {
		return err
	}
	c.store.RemoveAll()
	for _, record := range records {
		c.Update(record)
	}
	c.populated = true
	return nil
}

// Update adds or updates an alert record in the cache.
func (c *AlertsCache) Update(record *core.Record) {
	systemID := record.GetString("system")
	if systemID == "" {
		return
	}
	systemStore, ok := c.store.GetOk(systemID)
	if !ok {
		systemStore = store.New(map[string]CachedAlertData{})
		c.store.Set(systemID, systemStore)
	}
	var ca CachedAlertData
	ca.PopulateFromRecord(record)
	systemStore.Set(record.Id, ca)
}

// Delete removes an alert record from the cache.
func (c *AlertsCache) Delete(record *core.Record) {
	systemID := record.GetString("system")
	if systemID == "" {
		return
	}
	if systemStore, ok := c.store.GetOk(systemID); ok {
		systemStore.Remove(record.Id)
	}
}

// GetSystemAlerts returns all alerts for the specified system, lazy-loading if necessary.
func (c *AlertsCache) GetSystemAlerts(systemID string) []CachedAlertData {
	systemStore, ok := c.store.GetOk(systemID)
	if !ok {
		// Populate cache for this system
		records, err := c.app.FindAllRecords("alerts", dbx.NewExp("system={:system}", dbx.Params{"system": systemID}))
		if err != nil {
			return nil
		}
		systemStore = store.New(map[string]CachedAlertData{})
		for _, record := range records {
			var ca CachedAlertData
			ca.PopulateFromRecord(record)
			systemStore.Set(record.Id, ca)
		}
		c.store.Set(systemID, systemStore)
	}
	all := systemStore.GetAll()
	alerts := make([]CachedAlertData, 0, len(all))
	for _, alert := range all {
		alerts = append(alerts, alert)
	}
	return alerts
}

// GetAlert returns a specific alert by its ID from the cache.
func (c *AlertsCache) GetAlert(systemID, alertID string) (CachedAlertData, bool) {
	if systemStore, ok := c.store.GetOk(systemID); ok {
		return systemStore.GetOk(alertID)
	}
	return CachedAlertData{}, false
}

// GetAlertsByName returns all alerts of a specific type for the specified system.
func (c *AlertsCache) GetAlertsByName(systemID, alertName string) []CachedAlertData {
	allAlerts := c.GetSystemAlerts(systemID)
	var alerts []CachedAlertData
	for _, record := range allAlerts {
		if record.Name == alertName {
			alerts = append(alerts, record)
		}
	}
	return alerts
}

// GetAlertsExcludingNames returns all alerts for the specified system excluding the given types.
func (c *AlertsCache) GetAlertsExcludingNames(systemID string, excludedNames ...string) []CachedAlertData {
	excludeMap := make(map[string]struct{})
	for _, name := range excludedNames {
		excludeMap[name] = struct{}{}
	}
	allAlerts := c.GetSystemAlerts(systemID)
	var alerts []CachedAlertData
	for _, record := range allAlerts {
		if _, excluded := excludeMap[record.Name]; !excluded {
			alerts = append(alerts, record)
		}
	}
	return alerts
}

// Refresh returns the latest cached copy for an alert snapshot if it still exists.
func (c *AlertsCache) Refresh(alert CachedAlertData) (CachedAlertData, bool) {
	if alert.Id == "" {
		return CachedAlertData{}, false
	}
	return c.GetAlert(alert.SystemID, alert.Id)
}
