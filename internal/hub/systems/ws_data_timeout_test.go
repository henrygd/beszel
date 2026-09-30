//go:build testing

package systems

import (
	"context"
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

// slowDataClient answers GetData only after release is closed, simulating an
// agent whose collection outlasts the hub's request timeout.
type slowDataClient struct {
	gws.BuiltinEventHandler
	release chan struct{}
}

func (c *slowDataClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	var req common.HubRequest[cbor.RawMessage]
	if err := cbor.Unmarshal(message.Bytes(), &req); err != nil || req.Action != common.GetData {
		return
	}
	<-c.release
	response, _ := cbor.Marshal(common.AgentResponse{Id: req.Id, SystemData: &esystem.CombinedData{}})
	_ = conn.WriteMessage(gws.OpcodeBinary, response)
}

func TestFetchDataTimeoutKeepsWebSocketOpen(t *testing.T) {
	originalTimeout := wsDataRequestTimeout
	wsDataRequestTimeout = 50 * time.Millisecond
	t.Cleanup(func() { wsDataRequestTimeout = originalTimeout })

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

	client := &slowDataClient{release: make(chan struct{})}
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

	_, err = sys.fetchDataFromAgent(common.DataRequestOptions{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, sys.WsConn.IsConnected(), "a slow collection must not close the connection")

	// The late response is discarded and the next request still succeeds.
	close(client.release)
	_, err = sys.fetchDataFromAgent(common.DataRequestOptions{})
	require.NoError(t, err)
}

// pingCountingClient never answers GetData and records the pings it receives,
// standing in for an agent whose collection outlasts the request timeout.
type pingCountingClient struct {
	gws.BuiltinEventHandler
	pings chan struct{}
}

func (c *pingCountingClient) OnPing(conn *gws.Conn, payload []byte) {
	_ = conn.WritePong(payload)
	select {
	case c.pings <- struct{}{}:
	default:
	}
}

func (c *pingCountingClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	// Swallow the request: this agent is still collecting.
	message.Close()
}

// A slow agent used to be disconnected between polls. The fetch gives up after
// wsDataRequestTimeout, but the hub's read deadline is longer and only traffic
// from the agent resets it, so nothing kept the connection alive in between.
// update() now pings on a failed fetch, which is what breaks the reconnect
// loop reported in #2294.
func TestUpdatePingsAgentAfterFailedFetch(t *testing.T) {
	originalTimeout := wsDataRequestTimeout
	wsDataRequestTimeout = 50 * time.Millisecond
	t.Cleanup(func() { wsDataRequestTimeout = originalTimeout })

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

	client := &pingCountingClient{pings: make(chan struct{}, 4)}
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

	err = sys.update()
	require.ErrorIs(t, err, context.DeadlineExceeded, "the fetch is expected to time out")

	select {
	case <-client.pings:
	case <-time.After(3 * time.Second):
		t.Fatal("a failed fetch must ping the agent to keep the connection alive")
	}

	require.True(t, sys.WsConn.IsConnected(), "the connection must survive a failed fetch")
}
