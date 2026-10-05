//go:build testing

package systems

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/henrygd/beszel/internal/entities/monitor"
	esystem "github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/hub/ws"
	"github.com/lxzan/gws"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

type blockedUpdateHub struct {
	stubHub
	entered  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (h *blockedUpdateHub) HandleNetworkMonitorAlerts(*core.Record, map[string]monitor.Result) error {
	close(h.entered)
	<-h.release
	defer close(h.finished)
	// This callback is still allowed to use the database after cancellation.
	_, err := h.FindRecordsByFilter("alerts", "", "", 1, 0)
	return err
}

func TestRemoveAllSystemsWaitsForUpdater(t *testing.T) {
	for _, removed := range []bool{false, true} {
		name := "active"
		if removed {
			name = "already removed"
		}
		t.Run(name, func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)
			h := &blockedUpdateHub{stubHub: stubHub{app}, entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
			release := sync.OnceFunc(func() { close(h.release) })
			// Release first if a setup assertion fails, then stop workers before the DB.
			sm := NewSystemManager(h)
			t.Cleanup(sm.RemoveAllSystems)
			t.Cleanup(release)

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
			client := &sequenceDataClient{responses: make(chan esystem.CombinedData, 1)}
			client.responses <- esystem.CombinedData{}
			conn, _, err := gws.NewClient(client, &gws.ClientOption{Addr: "ws" + strings.TrimPrefix(server.URL, "http")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.NetConn().Close() })
			go conn.ReadLoop()
			select {
			case sys.WsConn = <-connections:
			case <-time.After(3 * time.Second):
				t.Fatal("websocket was not established")
			}
			sys.Host = "127.0.0.1"
			require.NoError(t, sm.AddSystem(sys))
			select {
			case <-h.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("updater did not reach network alerts")
			}
			if removed {
				require.NoError(t, sm.RemoveSystem(sys.Id))
			}
			done := make(chan struct{})
			go func() { sm.RemoveAllSystems(); close(done) }()
			<-sys.ctx.Done()
			select {
			case <-done:
				t.Error("cleanup returned while an updater still needed the database")
			case <-time.After(100 * time.Millisecond):
			}
			release()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("cleanup did not finish after the updater was released")
			}
			<-h.finished
			require.Zero(t, sm.GetSystemCount())
		})
	}
}
