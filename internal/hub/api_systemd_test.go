package hub_test

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/gliderlabs/ssh"
	"github.com/henrygd/beszel/internal/common"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/require"
)

func systemdInfoTestHandler(t *testing.T) (http.Handler, string, string, string, *atomic.Int32) {
	t.Helper()
	hub, handler := firstUserTestMux(t)
	t.Cleanup(hub.Cleanup)
	owner, err := beszelTests.CreateUserWithRole(hub, "owner@example.com", "password123", "user")
	require.NoError(t, err)
	other, err := beszelTests.CreateUserWithRole(hub, "other@example.com", "password123", "user")
	require.NoError(t, err)
	ownerToken, err := owner.NewAuthToken()
	require.NoError(t, err)
	otherToken, err := other.NewAuthToken()
	require.NoError(t, err)

	var requests atomic.Int32
	agent := &ssh.Server{Version: "beszel_0.20.0", Handler: func(s ssh.Session) {
		var request common.HubRequest[common.SystemdInfoRequest]
		if err := cbor.NewDecoder(s).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Action != common.GetSystemdInfo || request.Data.ServiceName != "private.service" {
			t.Errorf("unexpected agent request: %+v", request)
			return
		}
		requests.Add(1)
		data, err := cbor.Marshal(map[string]any{"Description": "private service details"})
		if err == nil {
			err = cbor.NewEncoder(s).Encode(common.AgentResponse{Data: data})
		}
		if err != nil {
			t.Error(err)
		}
	}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = agent.Close() })
	go func() { _ = agent.Serve(listener) }()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	record, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name": "test-system", "host": host, "port": port, "users": []string{owner.Id},
	})
	require.NoError(t, err)
	system, err := hub.GetSystemManager().GetSystem(record.Id)
	require.NoError(t, err)
	system.StopUpdater()
	_, err = beszelTests.CreateRecord(hub, "systemd_services", map[string]any{
		"system": record.Id, "name": "private.service", "state": 0, "sub": 1,
	})
	require.NoError(t, err)
	return handler, "/api/beszel/systemd/info?system=" + record.Id + "&service=private.service", ownerToken, otherToken, &requests
}

func TestSystemdInfoCachePolicy(t *testing.T) {
	t.Setenv("SHARE_ALL_SYSTEMS", "false")
	t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "false")
	handler, path, owner, other, requests := systemdInfoTestHandler(t)
	for _, tc := range []struct {
		name, token, shareAll string
		status                int
	}{
		{"owner", owner, "false", http.StatusOK},
		{"other user", other, "false", http.StatusNotFound},
		{"unauthenticated", "", "false", http.StatusUnauthorized},
		{"shared system", other, "true", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SHARE_ALL_SYSTEMS", tc.shareAll)
			t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", tc.shareAll)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", tc.token)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			response := recorder.Result()
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, tc.status, response.StatusCode)
			if tc.status == http.StatusOK {
				require.JSONEq(t, `{"details":{"Description":"private service details"}}`, string(body))
				require.Equal(t, "private, max-age=60", response.Header.Get("Cache-Control"))
				require.Contains(t, response.Header.Values("Vary"), "Authorization")
			} else {
				require.NotContains(t, string(body), "private service details")
				require.NotContains(t, response.Header.Get("Cache-Control"), "public")
			}
		})
	}
	require.EqualValues(t, 2, requests.Load(), "only authorised requests should reach the agent")
}
