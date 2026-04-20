package hub

import (
	"github.com/henrygd/beszel/internal/entities/probe"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/pocketbase/pocketbase/core"
)

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
		key := config.Key()
		id := systems.MakeStableHashId(systemID, key)
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
