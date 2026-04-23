package systems

import (
	"context"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/probe"
)

// SyncNetworkProbes sends probe configurations to the agent.
func (sys *System) SyncNetworkProbes(configs []probe.Config) error {
	_, err := sys.syncNetworkProbes(probe.SyncRequest{Action: probe.SyncActionReplace, Configs: configs})
	return err
}

// UpsertNetworkProbe sends a single probe configuration change to the agent.
func (sys *System) UpsertNetworkProbe(config probe.Config, runNow bool) (*probe.Result, error) {
	resp, err := sys.syncNetworkProbes(probe.SyncRequest{
		Action: probe.SyncActionUpsert,
		Config: config,
		RunNow: runNow,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 {
		return nil, nil
	}
	result := resp.Result
	return &result, nil
}

// DeleteNetworkProbe removes a single probe task from the agent.
func (sys *System) DeleteNetworkProbe(id string) error {
	_, err := sys.syncNetworkProbes(probe.SyncRequest{
		Action: probe.SyncActionDelete,
		Config: probe.Config{ID: id},
	})
	return err
}

func (sys *System) syncNetworkProbes(req probe.SyncRequest) (probe.SyncResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result probe.SyncResponse
	return result, sys.request(ctx, common.SyncNetworkProbes, req, &result)
}
