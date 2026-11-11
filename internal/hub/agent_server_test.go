//go:build testing

package hub

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAgentListenAddr(t *testing.T) {
	testCases := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "empty disables the server", value: "", want: ""},
		{name: "blank disables the server", value: "  ", want: ""},
		{name: "bare port listens on all interfaces", value: "8091", want: ":8091"},
		{name: "surrounding whitespace is trimmed", value: " 8091 ", want: ":8091"},
		{name: "port with empty host", value: ":8091", want: ":8091"},
		{name: "ipv4 host and port", value: "127.0.0.1:8091", want: "127.0.0.1:8091"},
		{name: "ipv6 host and port", value: "[::1]:8091", want: "[::1]:8091"},
		{name: "hostname and port", value: "localhost:8091", want: "localhost:8091"},
		{name: "port zero is rejected", value: "0", wantErr: true},
		{name: "port above range is rejected", value: "65536", wantErr: true},
		{name: "negative port is rejected", value: "-1", wantErr: true},
		{name: "non-numeric port is rejected", value: "http", wantErr: true},
		{name: "host without port is rejected", value: "localhost:", wantErr: true},
		{name: "bare ipv6 without port is rejected", value: "::1", wantErr: true},
		{name: "non-numeric port with host is rejected", value: "127.0.0.1:abc", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAgentListenAddr(tc.value)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAgentServerHandlerOnlyServesAgentConnect(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)

	handler := hub.newAgentServerHandler()

	testCases := []struct {
		name   string
		method string
		path   string
		header map[string]string
		status int
	}{
		{name: "agent-connect without headers is bad request", method: http.MethodGet, path: agentConnectPath, status: http.StatusBadRequest},
		{
			name:   "agent-connect with unknown token is unauthorized",
			method: http.MethodGet,
			path:   agentConnectPath,
			header: map[string]string{"X-Token": "unknown-token", "X-Beszel": "0.18.0"},
			status: http.StatusUnauthorized,
		},
		{name: "agent-connect only accepts GET", method: http.MethodPost, path: agentConnectPath, status: http.StatusMethodNotAllowed},
		{name: "web UI is not served", method: http.MethodGet, path: "/", status: http.StatusNotFound},
		{name: "API is not served", method: http.MethodGet, path: "/api/health", status: http.StatusNotFound},
		{name: "collections API is not served", method: http.MethodGet, path: "/api/collections/users/records", status: http.StatusNotFound},
		{name: "other beszel routes are not served", method: http.MethodGet, path: "/api/beszel/info", status: http.StatusNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			assert.Equal(t, tc.status, recorder.Code)
		})
	}
}

func TestStartAgentServer(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)

	se := &core.ServeEvent{App: testApp}

	t.Run("not started when AGENT_PORT is unset", func(t *testing.T) {
		t.Setenv("AGENT_PORT", "")
		require.NoError(t, hub.startAgentServer(se))
	})

	t.Run("invalid AGENT_PORT returns an error", func(t *testing.T) {
		t.Setenv("AGENT_PORT", "not-a-port")
		assert.Error(t, hub.startAgentServer(se))
	})

	t.Run("port already in use returns an error", func(t *testing.T) {
		busy, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer busy.Close()
		t.Setenv("AGENT_PORT", busy.Addr().String())
		assert.Error(t, hub.startAgentServer(se))
	})

	t.Run("serves only the agent endpoint and stops on terminate", func(t *testing.T) {
		// find a free port
		probe, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := probe.Addr().String()
		require.NoError(t, probe.Close())

		t.Setenv("BESZEL_HUB_AGENT_PORT", addr)
		require.NoError(t, hub.startAgentServer(se))

		client := &http.Client{Timeout: 3 * time.Second}

		res, err := client.Get("http://" + addr + agentConnectPath)
		require.NoError(t, err)
		res.Body.Close()
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)

		res, err = client.Get("http://" + addr + "/api/health")
		require.NoError(t, err)
		res.Body.Close()
		assert.Equal(t, http.StatusNotFound, res.StatusCode)

		require.NoError(t, testApp.OnTerminate().Trigger(&core.TerminateEvent{App: testApp}))

		_, err = client.Get("http://" + addr + agentConnectPath)
		assert.Error(t, err, "server should stop accepting connections after terminate")
	})
}
