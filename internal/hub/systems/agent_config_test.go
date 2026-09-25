//go:build testing

package systems

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/agentconfig"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/hub/ws"
	"github.com/lxzan/gws"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

func TestAgentConfigSyncSkipsOlderAgents(t *testing.T) {
	for _, version := range []string{"0.0.0", "0.18.0", "0.19.0"} {
		t.Run(version, func(t *testing.T) {
			// No manager or transport: attempting to send anything would fail.
			sys := &System{agentVersion: semver.MustParse(version)}
			require.NoError(t, sys.syncAgentConfig())
		})
	}
}

// connectAgentConfigClient wires a fake WebSocket agent to a system manager and
// returns the client whose configs channel receives every agent config sync.
func connectAgentConfigClient(t *testing.T, sm *SystemManager, systemId string) *monitorSyncClient {
	t.Helper()
	version := semver.MustParse("0.20.0")
	connections := make(chan *ws.WsConn, 1)
	upgrader := gws.NewUpgrader(&monitorSyncServer{}, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r)
		if err != nil {
			t.Error(err)
			return
		}
		wsConn := ws.NewWsConnection(conn, version)
		conn.Session().Store("wsConn", wsConn)
		connections <- wsConn
		conn.ReadLoop()
	}))
	t.Cleanup(server.Close)

	client := &monitorSyncClient{
		requests: make(chan common.HubRequest[monitor.SyncRequest], 4),
		configs:  make(chan agentconfig.Config, 4),
	}
	conn, _, err := gws.NewClient(client, &gws.ClientOption{Addr: "ws" + strings.TrimPrefix(server.URL, "http")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.NetConn().Close() })
	go conn.ReadLoop()
	select {
	case wsConn := <-connections:
		require.NoError(t, sm.AddWebSocketSystem(systemId, version, wsConn))
	case <-time.After(3 * time.Second):
		t.Fatal("websocket connection was not established")
	}
	return client
}

func receiveAgentConfig(t *testing.T, client *monitorSyncClient) agentconfig.Config {
	t.Helper()
	select {
	case cfg := <-client.configs:
		return cfg
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not receive an agent config sync")
		return agentconfig.Config{}
	}
}

// saveSystemConfig creates or updates a system's system_config record.
func saveSystemConfig(t *testing.T, app core.App, systemId, excludeContainers string) *core.Record {
	t.Helper()
	record, err := app.FindFirstRecordByFilter("system_config", "system = {:s}", dbx.Params{"s": systemId})
	if err != nil {
		collection, err := app.FindCachedCollectionByNameOrId("system_config")
		require.NoError(t, err)
		record = core.NewRecord(collection)
		record.Set("system", systemId)
	}
	record.Set("exclude_containers", excludeContainers)
	require.NoError(t, app.Save(record))
	return record
}

func TestAgentConfigSyncOnConnectAndUpdate(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	record, err := app.FindRecordById("systems", sys.Id)
	require.NoError(t, err)
	// Suppress unrelated system-stat requests while exercising config sync.
	record.Set("status", paused)
	require.NoError(t, app.SaveNoValidate(record))
	configRecord := saveSystemConfig(t, app, sys.Id, "web-*, db")

	sm := NewSystemManager(stubHub{app})
	// Initialize would also load systems and SSH config; only the record hooks matter here.
	sm.bindEventHooks()
	t.Cleanup(func() {
		sm.cancel()
		_ = sm.RemoveSystem(sys.Id)
		sm.smartFetchMap.StopCleaner()
		sm.zfsFetchMap.StopCleaner()
	})

	client := connectAgentConfigClient(t, sm, sys.Id)
	require.Equal(t, []string{"web-*", "db"}, receiveAgentConfig(t, client).ExcludeContainers)

	system, err := sm.GetSystem(sys.Id)
	require.NoError(t, err)
	require.False(t, system.configNeedsSync.Load(), "successful sync clears the flag")

	// Changing the record marks the system for sync. The system is paused, so
	// the push waits for the next sync but the flag proves the change was seen.
	configRecord.Set("exclude_containers", "cache")
	require.NoError(t, app.Save(configRecord))
	require.True(t, system.configNeedsSync.Load())
	system.syncPendingAgentConfig()
	require.Equal(t, []string{"cache"}, receiveAgentConfig(t, client).ExcludeContainers)
	require.False(t, system.configNeedsSync.Load())

	// Deleting the record still sends an empty replacement.
	require.NoError(t, app.Delete(configRecord))
	require.True(t, system.configNeedsSync.Load())
	system.syncPendingAgentConfig()
	require.Empty(t, receiveAgentConfig(t, client).ExcludeContainers)

	// Creating it again marks the system for sync once more.
	saveSystemConfig(t, app, sys.Id, "again")
	require.True(t, system.configNeedsSync.Load())
}

// A system with no system_config record must still sync, sending an empty
// config to clear anything the agent retained.
func TestAgentConfigSyncNoRecord(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	record, err := app.FindRecordById("systems", sys.Id)
	require.NoError(t, err)
	record.Set("status", paused)
	require.NoError(t, app.SaveNoValidate(record))

	sm := NewSystemManager(stubHub{app})
	t.Cleanup(func() {
		sm.cancel()
		_ = sm.RemoveSystem(sys.Id)
		sm.smartFetchMap.StopCleaner()
		sm.zfsFetchMap.StopCleaner()
	})

	client := connectAgentConfigClient(t, sm, sys.Id)
	require.Empty(t, receiveAgentConfig(t, client).ExcludeContainers)

	system, err := sm.GetSystem(sys.Id)
	require.NoError(t, err)
	require.False(t, system.configNeedsSync.Load(), "sync without a record must succeed")
}

func TestLoadAgentConfig(t *testing.T) {
	sys, app := newTestSystemWithHub(t)

	t.Run("no record", func(t *testing.T) {
		cfg, err := loadAgentConfig(app, sys.Id)
		require.NoError(t, err)
		require.Empty(t, cfg.ExcludeContainers)
	})

	t.Run("trims and drops blanks", func(t *testing.T) {
		saveSystemConfig(t, app, sys.Id, " web-* , ,db,")
		cfg, err := loadAgentConfig(app, sys.Id)
		require.NoError(t, err)
		require.Equal(t, []string{"web-*", "db"}, cfg.ExcludeContainers)
	})

	t.Run("reads service patterns", func(t *testing.T) {
		record := saveSystemConfig(t, app, sys.Id, "web-*, db")
		record.Set("service_patterns", " nginx, ,docker*,")
		require.NoError(t, app.Save(record))
		cfg, err := loadAgentConfig(app, sys.Id)
		require.NoError(t, err)
		require.Equal(t, []string{"nginx", "docker*"}, cfg.ServicePatterns)
		record.Set("nics", " -veth*, docker0 ")
		require.NoError(t, app.Save(record))
		cfg, err = loadAgentConfig(app, sys.Id)
		require.NoError(t, err)
		require.Equal(t, "-veth*, docker0", cfg.Nics)
		require.Equal(t, []string{"web-*", "db"}, cfg.ExcludeContainers, "settings are independent")
	})

	t.Run("only reads the requested system", func(t *testing.T) {
		collection, err := app.FindCachedCollectionByNameOrId("systems")
		require.NoError(t, err)
		other := core.NewRecord(collection)
		other.Set("name", "other")
		other.Set("host", "127.0.0.2")
		other.Set("port", "45876")
		require.NoError(t, app.SaveNoValidate(other))
		saveSystemConfig(t, app, other.Id, "other-*")

		cfg, err := loadAgentConfig(app, sys.Id)
		require.NoError(t, err)
		require.Equal(t, []string{"web-*", "db"}, cfg.ExcludeContainers)
		cfg, err = loadAgentConfig(app, other.Id)
		require.NoError(t, err)
		require.Equal(t, []string{"other-*"}, cfg.ExcludeContainers)
	})
}

func TestSystemConfigOnePerSystem(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	saveSystemConfig(t, app, sys.Id, "a")
	collection, err := app.FindCachedCollectionByNameOrId("system_config")
	require.NoError(t, err)
	duplicate := core.NewRecord(collection)
	duplicate.Set("system", sys.Id)
	require.Error(t, app.Save(duplicate), "the unique index allows one record per system")
}
