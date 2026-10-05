package systems

import (
	"context"
	"fmt"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/monitor"
)

// syncPendingNetworkMonitors runs on WebSocket connect and after successful stats
// fetches. Failed syncs retry on the next update without taking the system down.
func (sys *System) syncPendingNetworkMonitors() {
	// A paused system keeps the flag, so its monitors sync once it resumes.
	if sys.GetStatus() == paused || !sys.monitorsNeedSync.Swap(false) {
		return
	}
	if err := sys.syncAllNetworkMonitors(); err != nil {
		sys.monitorsNeedSync.Store(true)
		sys.manager.hub.Logger().Warn("failed to sync monitors to agent", "system", sys.Id, "err", err)
	}
}

func (sys *System) syncAllNetworkMonitors() error {
	configs, err := sys.manager.GetMonitorConfigsForSystem(sys.Id)
	if err != nil {
		return fmt.Errorf("failed to load monitors: %w", err)
	}
	// An empty set must also replace probes retained across a disconnect.
	return sys.SyncNetworkMonitors(configs)
}

// suspendNetworkMonitors stops all monitors on the agent of a paused system. Pausing
// keeps a WebSocket connection open, so the agent can't tell it should stop on its
// own; SSH agents suspend them when the hub closes the connection. The first update
// after the system resumes syncs them again.
func (sys *System) suspendNetworkMonitors() {
	sys.monitorsNeedSync.Store(true)
	if sys.WsConn == nil || !sys.WsConn.IsConnected() {
		return
	}
	if err := sys.SyncNetworkMonitors(nil); err != nil {
		sys.manager.hub.Logger().Warn("failed to suspend monitors on agent", "system", sys.Id, "err", err)
	}
	// A resume may have synced while this request was in flight and been undone by
	// it, so make sure the next update syncs again.
	if sys.GetStatus() != paused {
		sys.monitorsNeedSync.Store(true)
	}
}

// SyncNetworkMonitors sends monitor configurations to the agent.
func (sys *System) SyncNetworkMonitors(configs []monitor.Config) error {
	_, err := sys.syncNetworkMonitors(monitor.SyncRequest{Action: monitor.SyncActionReplace, Configs: configs})
	return err
}

// UpsertNetworkMonitor sends a single monitor configuration change to the agent.
// A paused system gets the change with the full sync once it resumes.
func (sys *System) UpsertNetworkMonitor(config monitor.Config, runNow bool) (*monitor.Result, error) {
	if sys.GetStatus() == paused {
		sys.monitorsNeedSync.Store(true)
		return nil, nil
	}
	resp, err := sys.syncNetworkMonitors(monitor.SyncRequest{
		Action: monitor.SyncActionUpsert,
		Config: config,
		RunNow: runNow,
	})
	if err != nil {
		return nil, err
	}
	if resp.Result == (monitor.Result{}) {
		return nil, nil
	}
	result := resp.Result
	return &result, nil
}

// DeleteNetworkMonitor removes a single monitor task from the agent.
func (sys *System) DeleteNetworkMonitor(id string) error {
	_, err := sys.syncNetworkMonitors(monitor.SyncRequest{
		Action: monitor.SyncActionDelete,
		Config: monitor.Config{ID: id},
	})
	return err
}

func (sys *System) syncNetworkMonitors(req monitor.SyncRequest) (monitor.SyncResponse, error) {
	if sys.getAgentVersion().LT(beszel.MinVersionNetworkMonitors) {
		return monitor.SyncResponse{}, nil
	}
	timeout := 5 * time.Second
	if req.Action == monitor.SyncActionUpsert && req.RunNow {
		// Allow the probe to finish, including a timeout result, while preserving
		// the normal request budget for transport and response handling.
		timeout += monitor.MaxProbeTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var result monitor.SyncResponse
	return result, sys.request(ctx, common.SyncNetworkMonitors, req, &result)
}
