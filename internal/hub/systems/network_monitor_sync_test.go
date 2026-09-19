//go:build testing

package systems

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/monitor"
	esystem "github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/hub/ws"
	"github.com/lxzan/gws"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

type monitorSyncClient struct {
	gws.BuiltinEventHandler
	requests chan common.HubRequest[monitor.SyncRequest]
	failSync atomic.Bool
}

func (c *monitorSyncClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	var req common.HubRequest[cbor.RawMessage]
	if err := cbor.Unmarshal(message.Bytes(), &req); err != nil {
		return
	}
	resp := common.AgentResponse{Id: req.Id}
	if req.Action == common.GetData {
		resp.SystemData = &esystem.CombinedData{}
	} else {
		var data monitor.SyncRequest
		if err := cbor.Unmarshal(req.Data, &data); err != nil {
			return
		}
		c.requests <- common.HubRequest[monitor.SyncRequest]{Id: req.Id, Action: req.Action, Data: data}
		if c.failSync.Load() {
			resp.Error = "test sync failure"
		} else {
			resp.Data, _ = cbor.Marshal(monitor.SyncResponse{})
		}
	}
	response, _ := cbor.Marshal(resp)
	_ = conn.WriteMessage(gws.OpcodeBinary, response)
}

// Avoid the production delayed disconnect notification; these tests explicitly
// remove each connection from the manager before reconnecting.
type monitorSyncServer struct{ ws.Handler }

func (*monitorSyncServer) OnClose(*gws.Conn, error) {}

func TestNetworkMonitorSyncSkipsOlderAgents(t *testing.T) {
	for _, version := range []string{"0.0.0", "0.18.0", "0.19.0"} {
		t.Run(version, func(t *testing.T) {
			// No transport: attempting to send any request would fail.
			sys := &System{agentVersion: semver.MustParse(version)}
			require.NoError(t, sys.SyncNetworkMonitors(nil))
			result, err := sys.UpsertNetworkMonitor(monitor.Config{ID: "test"}, true)
			require.NoError(t, err)
			require.Nil(t, result)
			require.NoError(t, sys.DeleteNetworkMonitor("test"))
		})
	}
}

func TestNetworkMonitorReconnectSync(t *testing.T) {
	for _, change := range []string{"delete", "disable", "retry"} {
		t.Run(change, func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)
			record, err := app.FindRecordById("systems", sys.Id)
			require.NoError(t, err)
			// Suppress unrelated system-stat requests while exercising reconnects.
			record.Set("status", paused)
			require.NoError(t, app.SaveNoValidate(record))
			collection, err := app.FindCachedCollectionByNameOrId("network_monitors")
			require.NoError(t, err)
			probe := core.NewRecord(collection)
			probe.Load(map[string]any{
				"system": sys.Id, "target": "localhost", "protocol": "tcp",
				"port": 80, "interval": 60, "enabled": true,
			})
			require.NoError(t, app.SaveNoValidate(probe))

			sm := NewSystemManager(stubHub{app})
			t.Cleanup(func() {
				sm.cancel()
				_ = sm.RemoveSystem(sys.Id)
				sm.smartFetchMap.StopCleaner()
				sm.zfsFetchMap.StopCleaner()
			})
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
			client := &monitorSyncClient{requests: make(chan common.HubRequest[monitor.SyncRequest], 2)}
			connect := func() monitor.SyncRequest {
				t.Helper()
				conn, _, err := gws.NewClient(client, &gws.ClientOption{Addr: "ws" + strings.TrimPrefix(server.URL, "http")})
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.NetConn().Close() })
				go conn.ReadLoop()
				select {
				case wsConn := <-connections:
					require.NoError(t, sm.AddWebSocketSystem(sys.Id, version, wsConn))
				case <-time.After(3 * time.Second):
					t.Fatal("websocket connection was not established")
				}
				select {
				case req := <-client.requests:
					require.Equal(t, common.SyncNetworkMonitors, req.Action)
					require.Equal(t, monitor.SyncActionReplace, req.Data.Action)
					return req.Data
				case <-time.After(3 * time.Second):
					t.Fatal("reconnected agent did not receive a monitor replacement")
					return monitor.SyncRequest{}
				}
			}

			client.failSync.Store(change == "retry")
			initial := connect()
			require.Len(t, initial.Configs, 1)
			require.Equal(t, probe.Id, initial.Configs[0].ID)
			if change == "retry" {
				system, err := sm.GetSystem(sys.Id)
				require.NoError(t, err)
				require.Eventually(t, system.monitorsNeedSync.Load, time.Second, time.Millisecond)
				// A second failed sync must not fail the stats fetch or clear pending state.
				_, err = system.fetchDataFromAgent(common.DataRequestOptions{})
				require.NoError(t, err)
				require.True(t, system.monitorsNeedSync.Load())
				require.Len(t, client.requests, 1)
				<-client.requests
				client.failSync.Store(false)
				_, err = system.fetchDataFromAgent(common.DataRequestOptions{})
				require.NoError(t, err)
				require.False(t, system.monitorsNeedSync.Load())
				require.Len(t, client.requests, 1)
				retry := <-client.requests
				require.Equal(t, monitor.SyncActionReplace, retry.Data.Action)
				require.Equal(t, initial.Configs, retry.Data.Configs)
				_, err = system.fetchDataFromAgent(common.DataRequestOptions{})
				require.NoError(t, err)
				require.Empty(t, client.requests, "successful sync must not repeat on every fetch")
				return
			}
			require.NoError(t, sm.RemoveSystem(sys.Id))
			if change == "delete" {
				require.NoError(t, app.Delete(probe))
			} else {
				probe.Set("enabled", false)
				require.NoError(t, app.SaveNoValidate(probe))
			}
			require.Empty(t, connect().Configs, "reconnect must clear the agent's previous probe")
		})
	}
}

func TestGetMonitorConfigsForSystemQueryError(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	_, err := app.DB().NewQuery("DROP TABLE network_monitors").Execute()
	require.NoError(t, err)
	_, err = sys.manager.GetMonitorConfigsForSystem(sys.Id)
	require.Error(t, err, "a failed query must not be treated as an empty monitor set")
}
