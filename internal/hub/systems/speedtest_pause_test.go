//go:build testing

package systems

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/speedtest"
	"github.com/henrygd/beszel/internal/hub/ws"
	"github.com/lxzan/gws"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

type speedtestSyncClient struct {
	gws.BuiltinEventHandler
	requests chan speedtest.SyncRequest
}

func (c *speedtestSyncClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	var req common.HubRequest[cbor.RawMessage]
	if err := cbor.Unmarshal(message.Bytes(), &req); err != nil || req.Action != common.SyncSpeedtests {
		return
	}
	var data speedtest.SyncRequest
	if err := cbor.Unmarshal(req.Data, &data); err != nil {
		return
	}
	c.requests <- data
	resp := common.AgentResponse{Id: req.Id}
	resp.Data, _ = cbor.Marshal(struct{}{})
	response, _ := cbor.Marshal(resp)
	_ = conn.WriteMessage(gws.OpcodeBinary, response)
}

// connectSpeedtestSyncClient attaches a WebSocket agent that records speedtest sync requests.
func connectSpeedtestSyncClient(t *testing.T, sys *System) *speedtestSyncClient {
	t.Helper()
	version := semver.MustParse("0.21.0")
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

	client := &speedtestSyncClient{requests: make(chan speedtest.SyncRequest, 2)}
	conn, _, err := gws.NewClient(client, &gws.ClientOption{Addr: "ws" + strings.TrimPrefix(server.URL, "http")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.NetConn().Close() })
	go conn.ReadLoop()
	select {
	case sys.WsConn = <-connections:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket connection was not established")
	}
	sys.setAgentVersion(version)
	return client
}

func receiveSpeedtestSync(t *testing.T, client *speedtestSyncClient) speedtest.SyncRequest {
	t.Helper()
	select {
	case req := <-client.requests:
		return req
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not receive a speedtest sync")
		return speedtest.SyncRequest{}
	}
}

func TestPauseSuspendsSpeedtests(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	col, err := app.FindCachedCollectionByNameOrId("speedtests")
	require.NoError(t, err)
	record := core.NewRecord(col)
	record.Load(map[string]any{"id": "speedtest1", "system": sys.Id, "interval": 60, "enabled": true})
	require.NoError(t, app.SaveNoValidate(record))
	client := connectSpeedtestSyncClient(t, sys)

	sys.swapStatus(paused)
	sys.suspendSpeedtests()
	req := receiveSpeedtestSync(t, client)
	require.Equal(t, monitor.SyncActionReplace, req.Action)
	require.Empty(t, req.Configs, "pausing must stop all speedtests on the agent")
	require.True(t, sys.speedtestsNeedSync.Load(), "resuming must sync speedtests again")

	sys.swapStatus(up)
	sys.syncPendingSpeedtests()
	req = receiveSpeedtestSync(t, client)
	require.Equal(t, monitor.SyncActionReplace, req.Action)
	require.Equal(t, []speedtest.Config{{ID: "speedtest1", Interval: 60}}, req.Configs)
	require.False(t, sys.speedtestsNeedSync.Load())
}

func TestPauseSuspendResyncsAfterRacingResume(t *testing.T) {
	sys, _ := newTestSystemWithHub(t)
	client := connectSpeedtestSyncClient(t, sys)

	// The system resumed before the suspend request finished, so the suspend may
	// have undone the resume's sync; the next update must sync again.
	sys.swapStatus(up)
	sys.suspendSpeedtests()
	receiveSpeedtestSync(t, client)
	require.True(t, sys.speedtestsNeedSync.Load())
}
