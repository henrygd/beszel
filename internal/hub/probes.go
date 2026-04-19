package hub

import (
	"github.com/pocketbase/pocketbase/core"
)

func bindNetworkProbesEvents(h *Hub) {
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
