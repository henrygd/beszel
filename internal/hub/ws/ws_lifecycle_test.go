//go:build testing

package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blang/semver"
	"github.com/henrygd/beszel/internal/common"
	"github.com/lxzan/gws"
	"github.com/stretchr/testify/require"
)

func TestWsConnConcurrentClose(t *testing.T) {
	connections := make(chan *WsConn, 1)
	serverDone := make(chan struct{})
	upgrader := gws.NewUpgrader(&Handler{}, &gws.ServerOption{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(serverDone)
		conn, err := upgrader.Upgrade(w, r)
		if err != nil {
			t.Error(err)
			return
		}
		ws := NewWsConnection(conn, semver.MustParse("0.12.10"), "")
		conn.Session().Store("wsConn", ws)
		connections <- ws
		conn.ReadLoop()
	}))
	defer server.Close()
	client, _, err := gws.NewClient(&gws.BuiltinEventHandler{}, &gws.ClientOption{
		Addr: "ws" + strings.TrimPrefix(server.URL, "http"),
	})
	require.NoError(t, err)
	defer client.WriteClose(1000, nil)
	ws := <-connections
	require.True(t, ws.IsConnected())

	readerReady := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		close(readerReady)
		for {
			select {
			case <-serverDone:
				return
			default:
				ws.IsConnected()
			}
		}
	}()
	<-readerReady
	require.NoError(t, client.WriteClose(1000, nil))
	<-readerDone

	require.False(t, ws.IsConnected())
	require.ErrorIs(t, ws.Ping(), gws.ErrConnClosed)
	require.ErrorIs(t, ws.sendMessage(common.HubRequest[any]{}), gws.ErrConnClosed)
	ws.Close(nil)
}
