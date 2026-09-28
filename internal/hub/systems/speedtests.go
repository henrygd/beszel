package systems

import (
	"context"
	"fmt"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/speedtest"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// syncPendingAgentConfigs syncs network monitor and speedtest configs that are
// pending after a (re)connect or an earlier failed sync.
func (sys *System) syncPendingAgentConfigs() {
	sys.syncPendingNetworkMonitors()
	sys.syncPendingSpeedtests()
}

// markAgentConfigsNeedSync flags a fresh connection as needing full config syncs.
func (sys *System) markAgentConfigsNeedSync() {
	sys.monitorsNeedSync.Store(true)
	sys.speedtestsNeedSync.Store(true)
}

// syncPendingSpeedtests runs on connect and after successful stats fetches.
// Failed syncs retry on the next update without taking the system down.
func (sys *System) syncPendingSpeedtests() {
	if !sys.speedtestsNeedSync.Swap(false) {
		return
	}
	configs, err := sys.manager.GetSpeedtestConfigsForSystem(sys.Id)
	if err == nil {
		// An empty set must also replace speedtests retained across a disconnect.
		err = sys.SyncSpeedtests(configs)
	}
	if err != nil {
		sys.speedtestsNeedSync.Store(true)
		sys.manager.hub.Logger().Warn("failed to sync speedtests to agent", "system", sys.Id, "err", err)
	}
}

// SyncSpeedtests replaces all speedtests on the agent with the given configs.
func (sys *System) SyncSpeedtests(configs []speedtest.Config) error {
	return sys.syncSpeedtests(speedtest.SyncRequest{Action: monitor.SyncActionReplace, Configs: configs})
}

// UpsertSpeedtest sends a single speedtest configuration change to the agent.
// With runNow, the agent starts a run whose result arrives with the next stats.
func (sys *System) UpsertSpeedtest(config speedtest.Config, runNow bool) error {
	return sys.syncSpeedtests(speedtest.SyncRequest{Action: monitor.SyncActionUpsert, Config: config, RunNow: runNow})
}

// DeleteSpeedtest removes a single speedtest from the agent.
func (sys *System) DeleteSpeedtest(id string) error {
	return sys.syncSpeedtests(speedtest.SyncRequest{Action: monitor.SyncActionDelete, Config: speedtest.Config{ID: id}})
}

func (sys *System) syncSpeedtests(req speedtest.SyncRequest) error {
	if sys.agentVersion.LT(beszel.MinVersionSpeedtests) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var resp struct{}
	return sys.request(ctx, common.SyncSpeedtests, req, &resp)
}

// GetSpeedtestConfigsForSystem returns all enabled speedtest configs for a system.
func (sm *SystemManager) GetSpeedtestConfigsForSystem(systemID string) ([]speedtest.Config, error) {
	var rows []struct {
		ID       string `db:"id"`
		ServerID uint32 `db:"server_id"`
		Interval uint32 `db:"interval"`
	}
	err := sm.hub.DB().
		NewQuery("SELECT id, server_id, interval FROM speedtests WHERE system = {:system} AND enabled = true").
		Bind(dbx.Params{"system": systemID}).
		All(&rows)
	configs := make([]speedtest.Config, len(rows))
	for i, row := range rows {
		configs[i] = speedtest.Config{ID: row.ID, ServerID: row.ServerID, Interval: row.Interval}
	}
	return configs, err
}

// updateSpeedtestRecords stores new speedtest results on their speedtests records
// and adds a speedtest_stats record for each run. Failed runs are stored with
// their error and no measurements, so they show as gaps in the charts. Results
// the hub has already stored are skipped by comparing run times.
func (sys *System) updateSpeedtestRecords(app core.App, results map[string]speedtest.Result) error {
	var statsCollection *core.Collection
	for id, result := range results {
		record, err := app.FindRecordById("speedtests", id)
		if err != nil {
			// The speedtest may have been deleted since the agent last synced.
			continue
		}
		if record.GetString("system") != sys.Id || result.RunAt <= int64(record.GetInt("last_run")) {
			continue
		}
		setSpeedtestResultFields(record, result)
		if err := app.SaveNoValidate(record); err != nil {
			return fmt.Errorf("failed to update speedtest %s: %w", id, err)
		}
		if statsCollection == nil {
			if statsCollection, err = app.FindCachedCollectionByNameOrId("speedtest_stats"); err != nil {
				return err
			}
		}
		stats := core.NewRecord(statsCollection)
		if result.Error == "" {
			stats.Load(speedtestMeasurements(result))
		}
		stats.Load(map[string]any{
			"system":      sys.Id,
			"speedtest":   id,
			"created":     result.RunAt,
			"server_id":   result.ServerID,
			"server_name": result.ServerName,
			"url":         result.URL,
			"error":       result.Error,
		})
		if err := app.SaveNoValidate(stats); err != nil {
			return fmt.Errorf("failed to save speedtest stats %s: %w", id, err)
		}
	}
	return nil
}

// setSpeedtestResultFields stores the latest result on a speedtests record.
// A failed run clears the measurements, so the record never shows results
// older than its latest run. The server name and location are kept, since
// they also describe the server a pinned speedtest uses.
func setSpeedtestResultFields(record *core.Record, result speedtest.Result) {
	record.Set("last_run", result.RunAt)
	record.Set("error", result.Error)
	record.Set("updated", time.Now().UTC().Format(types.DefaultDateLayout))
	record.Load(speedtestMeasurements(result))
	if result.Error != "" {
		record.Set("url", "")
		return
	}
	record.Set("server_name", result.ServerName)
	record.Set("server_location", result.ServerLocation)
	record.Set("isp", result.ISP)
	record.Set("url", result.URL)
}

// speedtestMeasurements returns the measured values of a result, which are
// stored on both speedtests and speedtest_stats records.
func speedtestMeasurements(result speedtest.Result) map[string]any {
	return map[string]any{
		"download":         result.Download,
		"upload":           result.Upload,
		"ping":             result.Ping,
		"jitter":           result.Jitter,
		"loss":             result.Loss,
		"download_latency": result.DownloadLatency.IQM,
		"download_jitter":  result.DownloadLatency.Jitter,
		"upload_latency":   result.UploadLatency.IQM,
		"upload_jitter":    result.UploadLatency.Jitter,
	}
}
