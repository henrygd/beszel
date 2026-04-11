package systems

import (
	"context"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/probe"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// SyncNetworkProbes sends probe configurations to the agent.
func (sys *System) SyncNetworkProbes(configs []probe.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result string
	return sys.request(ctx, common.SyncNetworkProbes, configs, &result)
}

// FetchNetworkProbeResults fetches probe results from the agent.
func (sys *System) FetchNetworkProbeResults() (map[string]probe.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var results map[string]probe.Result
	err := sys.request(ctx, common.GetNetworkProbeResults, nil, &results)
	return results, err
}

// hasEnabledProbes returns true if this system has any enabled network probes.
func (sys *System) hasEnabledProbes() bool {
	count, err := sys.manager.hub.CountRecords("network_probes",
		dbx.NewExp("system = {:system} AND enabled = true", dbx.Params{"system": sys.Id}))
	return err == nil && count > 0
}

// fetchAndSaveProbeResults fetches probe results and saves them to the database.
func (sys *System) fetchAndSaveProbeResults() {
	hub := sys.manager.hub

	results, err := sys.FetchNetworkProbeResults()
	if err != nil || len(results) == 0 {
		return
	}

	collection, err := hub.FindCachedCollectionByNameOrId("network_probe_stats")
	if err != nil {
		return
	}

	record := core.NewRecord(collection)
	record.Set("system", sys.Id)
	record.Set("stats", results)
	record.Set("type", "1m")

	if err := hub.SaveNoValidate(record); err != nil {
		hub.Logger().Warn("failed to save probe stats", "system", sys.Id, "err", err)
	}
}
