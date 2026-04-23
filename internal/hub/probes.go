package hub

import (
	"strconv"

	"github.com/henrygd/beszel/internal/entities/probe"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/pocketbase/pocketbase/core"
)

// generateProbeID creates a stable hash ID for a probe based on its configuration and the system it belongs to.
func generateProbeID(systemId string, config probe.Config) string {
	intervalStr := strconv.FormatUint(uint64(config.Interval), 10)
	portStr := strconv.FormatUint(uint64(config.Port), 10)
	return systems.MakeStableHashId(systemId, config.Protocol, config.Target, portStr, intervalStr)
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
	hub.OnRecordCreateRequest("network_probes").BindFunc(func(e *core.RecordRequestEvent) error {
		err := e.Next()
		if err != nil {
			return err
		}
		if !e.Record.GetBool("enabled") {
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
		return nil
	})

	hub.OnRecordUpdateRequest("network_probes").BindFunc(func(e *core.RecordRequestEvent) error {
		err := e.Next()
		if err != nil {
			return err
		}
		if e.Record.GetBool("enabled") {
			_, err = hub.upsertNetworkProbe(e.Record, false)
		} else {
			err = hub.deleteNetworkProbe(e.Record)
		}
		if err != nil {
			hub.Logger().Warn("failed to sync updated probe to agent", "system", e.Record.GetString("system"), "probe", e.Record.Id, "err", err)
		}
		return nil
	})

	// sync probe to agent on delete
	hub.OnRecordDeleteRequest("network_probes").BindFunc(func(e *core.RecordRequestEvent) error {
		err := e.Next()
		if err != nil {
			return err
		}
		if err := hub.deleteNetworkProbe(e.Record); err != nil {
			hub.Logger().Warn("failed to delete probe on agent", "system", e.Record.GetString("system"), "probe", e.Record.Id, "err", err)
		}
		return nil
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
	record.Set("res", result.Get(0))
	record.Set("resAvg1h", result.Get(1))
	record.Set("resMin1h", result.Get(2))
	record.Set("resMax1h", result.Get(3))
	record.Set("loss1h", result.Get(4))
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
