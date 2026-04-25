package hub

import (
	"time"

	"github.com/henrygd/beszel/internal/entities/probe"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// generateProbeID creates a stable hash ID for a probe based on its configuration and the system it belongs to.
func generateProbeID(systemId string, config probe.Config) string {
	return systems.MakeStableHashId(systemId, config.Target, config.Protocol)
}

// bindNetworkProbesEvents keeps probe records and agent probe state in sync.
func bindNetworkProbesEvents(hub *Hub) {
	// on create, make sure the id is set to a stable hash
	hub.OnRecordCreate("network_probes").BindFunc(func(e *core.RecordEvent) error {
		systemID := e.Record.GetString("system")
		config := probeConfigFromRecord(e.Record)
		id := generateProbeID(systemID, *config)
		e.Record.Set("id", id)
		return e.Next()
	})

	// sync probe to agent on creation and persist the first result immediately when available
	hub.OnRecordAfterCreateSuccess("network_probes").BindFunc(func(e *core.RecordEvent) error {
		err := e.Next()
		if err != nil {
			return err
		}
		if !e.Record.GetBool("enabled") {
			return nil
		}
		// if system connected, run the probe immediately
		// if not, return and wait for the system to connect and sync probes then
		system, err := hub.sm.GetSystem(e.Record.GetString("system"))
		if err != nil || system.Status != "up" {
			return nil
		}
		result, err := hub.upsertNetworkProbe(e.Record, true)
		if err != nil {
			hub.Logger().Warn("failed to sync probe to agent", "system", e.Record.GetString("system"), "probe", e.Record.Id, "err", err)
			return nil
		}
		if result == nil {
			return nil
		}
		setProbeResultFields(e.Record, *result)
		if err := e.App.SaveNoValidate(e.Record); err != nil {
			hub.Logger().Warn("failed to save initial probe result", "system", e.Record.GetString("system"), "probe", e.Record.Id, "err", err)
		}
		return e.Next()
	})

	// On API update requests, if the probe config changed in a way that requires a new ID, we will create a new
	// record with the new ID and delete the old one. Otherwise, we will just update the existing probe on the agent.
	hub.OnRecordUpdateRequest("network_probes").BindFunc(func(e *core.RecordRequestEvent) error {
		systemID := e.Record.GetString("system")
		ID := generateProbeID(systemID, *probeConfigFromRecord(e.Record))
		if ID != e.Record.Id {
			newRecord := copyProbeToNewRecord(e.Record, ID)
			if err := e.App.Save(newRecord); err != nil {
				return err
			}
			if err := e.App.Delete(e.Record); err != nil {
				return err
			}
			return nil
		}
		err := e.Next()
		if e.Record.GetBool("enabled") {
			var result *probe.Result
			runNow := !e.Record.Original().GetBool("enabled")
			result, err = hub.upsertNetworkProbe(e.Record, runNow)
			if result != nil {
				setProbeResultFields(e.Record, *result)
				_ = e.App.SaveNoValidate(e.Record)
			}
		} else {
			err = hub.deleteNetworkProbe(e.Record)
		}
		if err != nil {
			hub.Logger().Warn("failed to sync updated probe", "system", e.Record.GetString("system"), "probe", e.Record.Id, "err", err)
		}
		return nil
	})

	// sync probe to agent on delete
	hub.OnRecordAfterDeleteSuccess("network_probes").BindFunc(func(e *core.RecordEvent) error {
		if err := hub.deleteNetworkProbe(e.Record); err != nil {
			hub.Logger().Warn("failed to delete probe on agent", "system", e.Record.GetString("system"), "probe", e.Record.Id, "err", err)
		}
		return e.Next()
	})
}

// probeConfigFromRecord builds a probe config from a network_probes record.
func probeConfigFromRecord(record *core.Record) *probe.Config {
	return &probe.Config{
		ID:       record.Id,
		Target:   record.GetString("target"),
		Protocol: record.GetString("protocol"),
		Port:     uint16(record.GetInt("port")),
		Interval: uint16(record.GetInt("interval")),
	}
}

// setProbeResultFields stores the latest probe result values on the record.
func setProbeResultFields(record *core.Record, result probe.Result) {
	now := time.Now().UTC()
	nowString := now.Format(types.DefaultDateLayout)
	record.Set("res", result.Get(0))
	record.Set("resAvg1h", result.Get(1))
	record.Set("resMin1h", result.Get(2))
	record.Set("resMax1h", result.Get(3))
	record.Set("loss1h", result.Get(4))
	record.Set("updated", nowString)
}

// copyProbeToNewRecord creates a new record with the same field values as the old one.
// This is used when the probe config changes in a way that requires a new ID, so we need
// to create a new record with the new ID and delete the old one.
func copyProbeToNewRecord(oldRecord *core.Record, newID string) *core.Record {
	collection := oldRecord.Collection()
	newRecord := core.NewRecord(collection)
	newRecord.Load(oldRecord.FieldsData())
	newRecord.Set("id", newID)
	return newRecord
}

// upsertNetworkProbe applies the record's probe config to the target system.
func (h *Hub) upsertNetworkProbe(record *core.Record, runNow bool) (*probe.Result, error) {
	systemID := record.GetString("system")
	system, err := h.sm.GetSystem(systemID)
	if err != nil {
		return nil, err
	}
	return system.UpsertNetworkProbe(*probeConfigFromRecord(record), runNow)
}

// deleteNetworkProbe removes the record's probe from the target system.
func (h *Hub) deleteNetworkProbe(record *core.Record) error {
	systemID := record.GetString("system")
	system, err := h.sm.GetSystem(systemID)
	if err != nil {
		return err
	}
	return system.DeleteNetworkProbe(record.Id)
}
