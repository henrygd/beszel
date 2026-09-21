package systems

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/agentconfig"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// notifyAgentConfigChanged flags the config for sync and pushes it now if the
// system is up. Systems that aren't up sync on their next connection.
func (sys *System) notifyAgentConfigChanged() {
	sys.configNeedsSync.Store(true)
	if sys.Status == up {
		go sys.syncPendingAgentConfig()
	}
}

// syncPendingAgentConfig runs on connect and after successful stats fetches.
// Failed syncs retry on the next update without taking the system down.
func (sys *System) syncPendingAgentConfig() {
	if !sys.configNeedsSync.Swap(false) {
		return
	}
	if err := sys.syncAgentConfig(); err != nil {
		sys.configNeedsSync.Store(true)
		sys.manager.hub.Logger().Warn("failed to sync config to agent", "system", sys.Id, "err", err)
	}
}

// syncAgentConfig sends the system's full agent config to the agent. An empty
// config must also be sent, since the agent retains the last config across a
// disconnect and the hub value may have been cleared in the meantime.
func (sys *System) syncAgentConfig() error {
	if sys.agentVersion.LT(beszel.MinVersionAgentConfig) {
		return nil
	}
	cfg, err := loadAgentConfig(sys.manager.hub, sys.Id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The agent replies with an empty payload, but transports require a destination.
	var resp struct{}
	return sys.request(ctx, common.SyncAgentConfig, cfg, &resp)
}

// loadAgentConfig reads a system's settings from its system_config record.
// A system without a record has an empty config.
func loadAgentConfig(app core.App, systemId string) (agentconfig.Config, error) {
	record, err := app.FindFirstRecordByFilter("system_config", "system = {:system}", dbx.Params{"system": systemId})
	if errors.Is(err, sql.ErrNoRows) {
		return agentconfig.Config{}, nil
	}
	if err != nil {
		return agentconfig.Config{}, fmt.Errorf("failed to load system config: %w", err)
	}
	return agentconfig.Config{
		ExcludeContainers: splitCommaList(record.GetString("exclude_containers")),
		ServicePatterns:   splitCommaList(record.GetString("service_patterns")),
	}, nil
}

// splitCommaList splits a comma-separated setting, dropping blanks.
func splitCommaList(value string) []string {
	var items []string
	for part := range strings.SplitSeq(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			items = append(items, part)
		}
	}
	return items
}
