//go:build testing

package systems

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/monitor"
	esystem "github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/hub/expirymap"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestSSHNetworkMonitorReconnectSync(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	sys.manager.zfsFetchMap = expirymap.New[zfsFetchState](time.Hour)
	t.Cleanup(sys.manager.zfsFetchMap.StopCleaner)
	sys.ctx = context.Background()
	sys.Status = up
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	config := &ssh.ServerConfig{NoClientAuth: true, ServerVersion: "SSH-2.0-beszel_0.20.0"}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	sys.Host, sys.Port, err = net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	sys.manager.sshConfig = &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second}
	t.Cleanup(sys.closeSSHConnection)
	requests := make(chan monitor.SyncRequest, 10)
	var failSync atomic.Bool
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				server, channels, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					_ = conn.Close()
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(reqs)
				for channel := range channels {
					ch, reqs, err := channel.Accept()
					if err != nil {
						return
					}
					go func() {
						defer ch.Close()
						for req := range reqs {
							if req.Type != "shell" {
								_ = req.Reply(false, nil)
								continue
							}
							_ = req.Reply(true, nil)
							var request common.HubRequest[cbor.RawMessage]
							if cbor.NewDecoder(ch).Decode(&request) != nil {
								return
							}
							response := common.AgentResponse{}
							switch request.Action {
							case common.GetData:
								response.SystemData = &esystem.CombinedData{}
							case common.SyncNetworkMonitors:
								var syncReq monitor.SyncRequest
								if cbor.Unmarshal(request.Data, &syncReq) != nil {
									return
								}
								requests <- syncReq
								if failSync.Load() {
									response.Error = "test sync failure"
								} else {
									response.Data, _ = cbor.Marshal(monitor.SyncResponse{})
								}
							}
							_ = cbor.NewEncoder(ch).Encode(response)
							_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							return
						}
					}()
				}
			}()
		}
	}()
	collection, err := app.FindCachedCollectionByNameOrId("network_monitors")
	require.NoError(t, err)
	probe := core.NewRecord(collection)
	probe.Load(map[string]any{"system": sys.Id, "target": "localhost", "protocol": "tcp", "port": 80, "interval": 60, "enabled": true})
	require.NoError(t, app.SaveNoValidate(probe))
	fetch := func() {
		t.Helper()
		_, err := sys.fetchDataFromAgent(common.DataRequestOptions{})
		require.NoError(t, err, "monitor sync failure must not fail stats fetching")
	}
	receive := func() monitor.SyncRequest {
		t.Helper()
		select {
		case req := <-requests:
			require.Equal(t, monitor.SyncActionReplace, req.Action)
			return req
		case <-time.After(time.Second):
			t.Fatal("missing full monitor sync")
			return monitor.SyncRequest{}
		}
	}
	fetch()
	require.Equal(t, probe.Id, receive().Configs[0].ID)
	require.False(t, sys.monitorsNeedSync.Load())
	fetch()
	require.Empty(t, requests, "steady-state fetch must not resync")

	// Simulate loss of the agent process/connection and its in-memory monitors.
	require.NoError(t, sys.client.Load().Close())
	fetch()
	require.Equal(t, probe.Id, receive().Configs[0].ID)
	require.False(t, sys.monitorsNeedSync.Load())

	// Failed replacements are retried on the next successful stats fetch.
	require.NoError(t, sys.client.Load().Close())
	failSync.Store(true)
	fetch()
	receive()
	require.True(t, sys.monitorsNeedSync.Load())
	failSync.Store(false)
	fetch()
	receive()
	require.False(t, sys.monitorsNeedSync.Load())

	probe.Set("enabled", false)
	require.NoError(t, app.SaveNoValidate(probe))
	require.NoError(t, sys.client.Load().Close())
	fetch()
	require.Empty(t, receive().Configs, "empty replacement must clear stale monitors")
}

func TestPendingNetworkMonitorSyncQueryFailure(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	_, err := app.DB().NewQuery("DROP TABLE network_monitors").Execute()
	require.NoError(t, err)
	sys.monitorsNeedSync.Store(true)
	sys.syncPendingNetworkMonitors()
	require.True(t, sys.monitorsNeedSync.Load())
}
