package systems

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/monitor"
)

// syncPendingNetworkMonitors runs on WebSocket connect and after successful stats
// fetches. Failed syncs retry on the next update without taking the system down.
func (sys *System) syncPendingNetworkMonitors() {
	if !sys.monitorsNeedSync.Swap(false) {
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

// SyncNetworkMonitors sends monitor configurations to the agent.
func (sys *System) SyncNetworkMonitors(configs []monitor.Config) error {
	_, err := sys.syncNetworkMonitors(monitor.SyncRequest{Action: monitor.SyncActionReplace, Configs: configs})
	return err
}

// UpsertNetworkMonitor sends a single monitor configuration change to the agent.
func (sys *System) UpsertNetworkMonitor(config monitor.Config, runNow bool) (*monitor.Result, error) {
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
	agentVersion := sys.getAgentVersion()
	if agentVersion.LT(beszel.MinVersionNetworkMonitors) {
		return monitor.SyncResponse{}, nil
	}
	// Older agents ignore the IP version and would probe with either family, so
	// don't send them those monitors rather than report misleading results.
	if agentVersion.LT(beszel.MinVersionMonitorIPVersion) {
		switch req.Action {
		case monitor.SyncActionUpsert:
			if req.Config.IPVersion != monitor.IPVersionAuto {
				return monitor.SyncResponse{}, fmt.Errorf("agent %s does not support IP version for monitors; requires %s", agentVersion, beszel.MinVersionMonitorIPVersion)
			}
		case monitor.SyncActionReplace:
			configs := withoutIPVersionConfigs(req.Configs)
			if skipped := len(req.Configs) - len(configs); skipped > 0 {
				sys.manager.hub.Logger().Warn("skipping monitors with an IP version the agent does not support", "system", sys.Id, "count", skipped, "agent", agentVersion.String(), "requires", beszel.MinVersionMonitorIPVersion.String())
			}
			req.Configs = configs
		}
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

// withoutIPVersionConfigs returns the configs that use monitor.IPVersionAuto,
// leaving the input unchanged.
func withoutIPVersionConfigs(configs []monitor.Config) []monitor.Config {
	return slices.DeleteFunc(slices.Clone(configs), func(config monitor.Config) bool {
		return config.IPVersion != monitor.IPVersionAuto
	})
}
