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

func bindNetworkProbesEvents(h *Hub) {
	// on create, make sure the id is set to a stable hash
	h.OnRecordCreate("network_probes").BindFunc(func(e *core.RecordEvent) error {
		systemID := e.Record.GetString("system")
		config := &probe.Config{
			Target:   e.Record.GetString("target"),
			Protocol: e.Record.GetString("protocol"),
			Port:     uint16(e.Record.GetInt("port")),
			Interval: uint16(e.Record.GetInt("interval")),
		}
		id := generateProbeID(systemID, *config)
		e.Record.Set("id", id)
		return e.Next()
	})

	// sync probe to agent on creation
	h.OnRecordAfterCreateSuccess("network_probes").BindFunc(func(e *core.RecordEvent) error {
		systemID := e.Record.GetString("system")
		h.syncProbesToAgent(systemID)
		return e.Next()
	})
	// sync probe to agent on delete
	h.OnRecordAfterDeleteSuccess("network_probes").BindFunc(func(e *core.RecordEvent) error {
		systemID := e.Record.GetString("system")
		h.syncProbesToAgent(systemID)
		return e.Next()
	})
	// TODO: if enabled changes, sync to agent
}

// syncProbesToAgent fetches enabled probes for a system and sends them to the agent.
func (h *Hub) syncProbesToAgent(systemID string) {
	system, err := h.sm.GetSystem(systemID)
	if err != nil {
		return
	}

	configs := h.sm.GetProbeConfigsForSystem(systemID)

	go func() {
		if err := system.SyncNetworkProbes(configs); err != nil {
			h.Logger().Warn("failed to sync probes to agent", "system", systemID, "err", err)
		}
	}()
}
