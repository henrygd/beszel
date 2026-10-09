//go:build testing

package systems

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

// silentAfterFirstPollAgent answers the first GetData and then goes quiet,
// modelling an agent whose collection outlasts the request timeout. It never
// answers a ping, because a real agent serves hub requests on its read loop and
// cannot reply while it is collecting. The fix therefore has to work without an
// answer, which it does: Ping() pushes the deadline out as it sends.
type silentAfterFirstPollAgent struct {
	gws.BuiltinEventHandler
	answered atomic.Bool
}

func (c *silentAfterFirstPollAgent) OnPing(*gws.Conn, []byte) {}

func (c *silentAfterFirstPollAgent) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	var req common.HubRequest[cbor.RawMessage]
	if err := cbor.Unmarshal(message.Bytes(), &req); err != nil || req.Action != common.GetData {
		return
	}
	if c.answered.CompareAndSwap(false, true) {
		response, _ := cbor.Marshal(common.AgentResponse{Id: req.Id, SystemData: &esystem.CombinedData{}})
		_ = conn.WriteMessage(gws.OpcodeBinary, response)
	}
}

// timelineScale shrinks the poll schedule so the test stays fast while keeping
// the production ratio between poll interval, request timeout and read deadline.
const timelineScale = 100

// A slow agent must survive repeated failed polls. The ping in update() only
// happens once the fetch gives up, which is interval + wsDataRequestTimeout into
// the cycle, so the read deadline has to outlast that. With a deadline below
// that sum the connection is already closed when the ping would go out, which is
// the reconnect loop from #2294. An agent silent for longer than
// maxAgentSilence is no longer pinged, so its connection is allowed to expire.
func TestSlowAgentKeepsConnectionAcrossPolls(t *testing.T) {
	tick := time.Duration(interval) * time.Millisecond / timelineScale

	tests := []struct {
		name          string
		deadline      time.Duration
		polls         int
		wantConnected bool
	}{
		// The first failed poll shows the ping lands, the second shows the ping
		// keeps re-arming the deadline.
		{"production deadline outlasts a failed poll", ws.Deadline() / timelineScale, 2, true},
		// 70s was the old value and sits below the 90s a failed poll needs.
		{"deadline below interval plus request timeout", 70 * time.Second / timelineScale, 2, false},
		// The third failed poll ends 210s after the last message, past
		// maxAgentSilence, so it skips the ping and the deadline closes the
		// connection before the fifth poll.
		{"agent silent past maxAgentSilence", ws.Deadline() / timelineScale, 5, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			previousDeadline := ws.SetDeadline(tc.deadline)
			t.Cleanup(func() { ws.SetDeadline(previousDeadline) })
			previousTimeout := wsDataRequestTimeout
			wsDataRequestTimeout = 30 * time.Second / timelineScale
			t.Cleanup(func() { wsDataRequestTimeout = previousTimeout })
			previousSilence := maxAgentSilence
			maxAgentSilence = 3 * time.Minute / timelineScale
			t.Cleanup(func() { maxAgentSilence = previousSilence })

			connections := make(chan *ws.WsConn, 1)
			// The real handler: OnClose has to run, or a dropped connection
			// would still look connected.
			upgrader := gws.NewUpgrader(&ws.Handler{}, &gws.ServerOption{})
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

			client := &silentAfterFirstPollAgent{}
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

			// A healthy poll. Its response is what last resets the deadline, so
			// it is the point the schedule below is measured from.
			_, err = sys.fetchDataFromAgent(common.DataRequestOptions{})
			require.NoError(t, err, "the first poll must succeed")
			pollZero := time.Now()

			for poll := 1; poll <= tc.polls; poll++ {
				time.Sleep(time.Until(pollZero.Add(time.Duration(poll) * tick)))
				if !sys.WsConn.IsConnected() {
					break
				}
				require.Error(t, sys.update(), "a silent agent must fail the poll")
			}

			require.Equal(t, tc.wantConnected, sys.WsConn.IsConnected())
		})
	}
}
