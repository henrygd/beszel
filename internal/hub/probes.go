package hub

import (
	"strconv"
	"time"

	"github.com/henrygd/beszel/internal/entities/probe"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// generateProbeID creates a stable hash ID for a probe based on its configuration and the system it belongs to.
func generateProbeID(systemId string, config probe.Config) string {
	args := []string{systemId, config.Target, config.Protocol}
	// only use port for TCP probes, since for other protocols it's not relevant as standalone value
	if config.Protocol == "tcp" {
		args = append(args, strconv.FormatUint(uint64(config.Port), 10))
	}
	return systems.MakeStableHashId(args...)
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
		// if not, return and wait for the system to connect and sync probes on reg schedule
		system, err := hub.sm.GetSystem(e.Record.GetString("system"))
		if err == nil && system.Status == "up" {
			go hub.upsertNetworkProbe(e.Record, true)
		}
		return err
	})

	// On API update requests, if the probe config changed in a way that requires a new ID, create a new
	// record with the new ID and delete the old one. Otherwise, just update the existing probe on the agent.
	hub.OnRecordUpdateRequest("network_probes").BindFunc(func(e *core.RecordRequestEvent) error {
		systemID := e.Record.GetString("system")
		// only tcp uses port - set other protocols port to zero
		if e.Record.GetString("protocol") != "tcp" {
			e.Record.Set("port", 0)
		}
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
			// if the probe is enabled, sync the updated config to the agent now
			runNow := !e.Record.Original().GetBool("enabled")
			err = hub.upsertNetworkProbe(e.Record, runNow)
		} else {
			// if the probe is paused, remove it from the agent
			err = hub.deleteNetworkProbe(e.Record)
		}
		if err != nil {
			hub.Logger().Warn("failed to sync updated probe", "system", systemID, "probe", e.Record.Id, "err", err)
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
	nowString := time.Now().UTC().Format(types.DefaultDateLayout)
	record.Set("res", result.AvgResponse)
	record.Set("resAvg1h", result.AvgResponse1h)
	record.Set("resMin1h", result.MinResponse1h)
	record.Set("resMax1h", result.MaxResponse1h)
	record.Set("loss1h", result.PacketLoss1h)
	record.Set("updated", nowString)
}

// copyProbeToNewRecord creates a new record with the same field values as the old one.
// This is used when the probe config changes in a way that requires a new ID, so we need
// to create a new record with the new ID and delete the old one.
func copyProbeToNewRecord(oldRecord *core.Record, newID string) *core.Record {
	collection := oldRecord.Collection()
	newRecord := core.NewRecord(collection)
	newRecord.Id = newID
	fields := []string{"system", "name", "target", "protocol", "port", "interval", "enabled"}
	for _, field := range fields {
		newRecord.Set(field, oldRecord.Get(field))
	}
	return newRecord
}

// upsertNetworkProbe creates or updates the record's probe on the target system. If runNow
// is true, it will also trigger an immediate probe run and update the record with the result.
func (h *Hub) upsertNetworkProbe(record *core.Record, runNow bool) error {
	systemID := record.GetString("system")
	system, err := h.sm.GetSystem(systemID)
	if err != nil {
		return err
	}
	result, err := system.UpsertNetworkProbe(*probeConfigFromRecord(record), runNow)
	if err != nil || result == nil {
		return err
	}
	setProbeResultFields(record, *result)
	return h.App.SaveNoValidate(record)
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
