package hub

import (
	"strconv"
	"time"

	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// generateMonitorID creates a stable hash ID for a monitor based on its configuration and the system it belongs to.
func generateMonitorID(systemId string, config monitor.Config) string {
	args := []string{systemId, config.Target, config.Protocol}
	// only use port for TCP monitors, since for other protocols it's not relevant as standalone value
	if config.Protocol == "tcp" {
		args = append(args, strconv.FormatUint(uint64(config.Port), 10))
	}
	return systems.MakeStableHashId(args...)
}

// bindNetworkMonitorsEvents keeps monitor records and agent monitor state in sync.
func bindNetworkMonitorsEvents(hub *Hub) {
	// on create, make sure the id is set to a stable hash
	hub.OnRecordCreate("network_monitors").BindFunc(func(e *core.RecordEvent) error {
		systemID := e.Record.GetString("system")
		config := monitorConfigFromRecord(e.Record)
		id := generateMonitorID(systemID, *config)
		e.Record.Set("id", id)
		return e.Next()
	})

	// sync monitor to agent on creation and persist the first result immediately when available
	hub.OnRecordAfterCreateSuccess("network_monitors").BindFunc(func(e *core.RecordEvent) error {
		err := e.Next()
		if err != nil {
			return err
		}
		if !e.Record.GetBool("enabled") {
			return nil
		}
		// If connected, run the monitor immediately. Paused systems may be absent
		// from the manager; their monitors will sync when they reconnect.
		system, err := hub.sm.GetSystem(e.Record.GetString("system"))
		if err == nil && system.Status == "up" {
			go hub.upsertNetworkMonitor(e.Record, true)
		}
		return nil
	})

	// On API update requests, if the monitor config changed in a way that requires a new ID, create a new
	// record with the new ID and delete the old one. Otherwise, just update the existing monitor on the agent.
	hub.OnRecordUpdateRequest("network_monitors").BindFunc(func(e *core.RecordRequestEvent) error {
		systemID := e.Record.GetString("system")
		// only tcp uses port - set other protocols port to zero
		if e.Record.GetString("protocol") != "tcp" {
			e.Record.Set("port", 0)
		}
		ID := generateMonitorID(systemID, *monitorConfigFromRecord(e.Record))
		if ID != e.Record.Id {
			newRecord := copyMonitorToNewRecord(e.Record, ID)
			if err := e.App.Save(newRecord); err != nil {
				return err
			}
			if err := e.App.Delete(e.Record); err != nil {
				return err
			}
			return nil
		}
		err := e.Next()
		if err != nil {
			return err
		}
		if e.Record.GetBool("enabled") {
			// if the monitor is enabled, sync the updated config to the agent now
			runNow := !e.Record.Original().GetBool("enabled")
			err = hub.upsertNetworkMonitor(e.Record, runNow)
		} else {
			// if the monitor is paused, remove it from the agent
			err = hub.deleteNetworkMonitor(e.Record)
		}
		if err != nil {
			hub.Logger().Warn("failed to sync updated monitor", "system", systemID, "monitor", e.Record.Id, "err", err)
		}
		return nil
	})

	// sync monitor to agent on delete
	hub.OnRecordAfterDeleteSuccess("network_monitors").BindFunc(func(e *core.RecordEvent) error {
		if err := hub.deleteNetworkMonitor(e.Record); err != nil {
			hub.Logger().Warn("failed to delete monitor on agent", "system", e.Record.GetString("system"), "monitor", e.Record.Id, "err", err)
		}
		return e.Next()
	})
}

// monitorConfigFromRecord builds a monitor config from a network_monitors record.
func monitorConfigFromRecord(record *core.Record) *monitor.Config {
	return &monitor.Config{
		ID:       record.Id,
		Target:   record.GetString("target"),
		Protocol: record.GetString("protocol"),
		Port:     uint16(record.GetInt("port")),
		Interval: uint16(record.GetInt("interval")),
	}
}

// setMonitorResultFields stores the latest monitor result values on the record.
func setMonitorResultFields(record *core.Record, result monitor.Result) {
	nowString := time.Now().UTC().Format(types.DefaultDateLayout)
	record.Set("res", result.AvgResponse)
	record.Set("resAvg1h", result.AvgResponse1h)
	record.Set("resMin1h", result.MinResponse1h)
	record.Set("resMax1h", result.MaxResponse1h)
	record.Set("loss1h", result.PacketLoss1h)
	record.Set("updated", nowString)
}

// copyMonitorToNewRecord creates a new record with the same field values as the old one.
// This is used when the monitor config changes in a way that requires a new ID, so we need
// to create a new record with the new ID and delete the old one.
func copyMonitorToNewRecord(oldRecord *core.Record, newID string) *core.Record {
	collection := oldRecord.Collection()
	newRecord := core.NewRecord(collection)
	newRecord.Id = newID
	fields := []string{"system", "target", "protocol", "port", "interval", "enabled"}
	for _, field := range fields {
		newRecord.Set(field, oldRecord.Get(field))
	}
	return newRecord
}

// upsertNetworkMonitor creates or updates the record's monitor on the target system. If runNow
// is true, it will also trigger an immediate monitor run and update the record with the result.
func (h *Hub) upsertNetworkMonitor(record *core.Record, runNow bool) error {
	systemID := record.GetString("system")
	system, err := h.sm.GetSystem(systemID)
	if err != nil {
		return err
	}
	result, err := system.UpsertNetworkMonitor(*monitorConfigFromRecord(record), runNow)
	if err != nil || result == nil {
		return err
	}
	setMonitorResultFields(record, *result)
	return h.App.SaveNoValidate(record)
}

// deleteNetworkMonitor removes the record's monitor from the target system.
func (h *Hub) deleteNetworkMonitor(record *core.Record) error {
	systemID := record.GetString("system")
	system, err := h.sm.GetSystem(systemID)
	if err != nil {
		return err
	}
	return system.DeleteNetworkMonitor(record.Id)
}
