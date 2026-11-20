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
	esystem "github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/hub/ws"
	"github.com/lxzan/gws"
	"github.com/stretchr/testify/require"
)

// sequenceDataClient answers each GetData request with the next queued payload.
type sequenceDataClient struct {
	gws.BuiltinEventHandler
	responses chan esystem.CombinedData
}

func (c *sequenceDataClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	var req common.HubRequest[cbor.RawMessage]
	if err := cbor.Unmarshal(message.Bytes(), &req); err != nil || req.Action != common.GetData {
		return
	}
	data, _ := cbor.Marshal(<-c.responses)
	response, _ := cbor.Marshal(common.AgentResponse{Id: req.Id, Data: data})
	_ = conn.WriteMessage(gws.OpcodeBinary, response)
}

// Fields the agent omits must not carry over from a previous response.
func TestFetchDataViaWebSocketDoesNotRetainOmittedFields(t *testing.T) {
	connections := make(chan *ws.WsConn, 1)
	upgrader := gws.NewUpgrader(&monitorSyncServer{}, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r)
		if err != nil {
			t.Error(err)
			return
		}
		wsConn := ws.NewWsConnection(conn, semver.MustParse("0.20.0"))
		conn.Session().Store("wsConn", wsConn)
		connections <- wsConn
		conn.ReadLoop()
	}))
	t.Cleanup(server.Close)

	client := &sequenceDataClient{responses: make(chan esystem.CombinedData, 2)}
	client.responses <- esystem.CombinedData{
		Details:                &esystem.Details{Hostname: "host"},
		SystemdServicesUpdated: true,
		Stats:                  esystem.Stats{Batteries: map[string]uint8{"BAT0": 50, "BAT1": 60}},
		Info:                   esystem.Info{WiFi: map[string]esystem.WiFi{"wlan0": {SSID: "home"}}},
	}
	client.responses <- esystem.CombinedData{
		Stats: esystem.Stats{Batteries: map[string]uint8{"BAT0": 40}},
	}
	conn, _, err := gws.NewClient(client, &gws.ClientOption{Addr: "ws" + strings.TrimPrefix(server.URL, "http")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.NetConn().Close() })
	go conn.ReadLoop()

	var sys *System
	select {
	case wsConn := <-connections:
		sys = &System{WsConn: wsConn}
	case <-time.After(3 * time.Second):
		t.Fatal("websocket connection was not established")
	}

	first, err := sys.fetchDataFromAgent(common.DataRequestOptions{})
	require.NoError(t, err)
	require.NotNil(t, first.Details)
	require.Len(t, first.Stats.Batteries, 2)

	second, err := sys.fetchDataFromAgent(common.DataRequestOptions{})
	require.NoError(t, err)
	require.Nil(t, second.Details)
	require.False(t, second.SystemdServicesUpdated)
	require.Equal(t, map[string]uint8{"BAT0": 40}, second.Stats.Batteries)
	require.Empty(t, second.Info.WiFi)

	// the first result is not mutated by the second fetch
	require.Len(t, first.Stats.Batteries, 2)
}
