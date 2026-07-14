//go:build testing

package agent

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureHostPort(t *testing.T) {
	assert.Equal(t, "1.1.1.1:443", ensureHostPort("1.1.1.1"))
	assert.Equal(t, "example.com:80", ensureHostPort("example.com:80"))
	assert.Equal(t, "[2001:db8::1]:443", ensureHostPort("[2001:db8::1]:443"))
	assert.Equal(t, "example.com:443", ensureHostPort("example.com"))
}

func TestTargetFromURL(t *testing.T) {
	assert.Equal(t, "hub.example.com:443", targetFromURL("https://hub.example.com"))
	assert.Equal(t, "hub.example.com:80", targetFromURL("http://hub.example.com"))
	assert.Equal(t, "hub.example.com:8090", targetFromURL("https://hub.example.com:8090/path"))
	assert.Equal(t, "10.0.0.1:8000", targetFromURL("10.0.0.1:8000"))
	assert.Equal(t, "", targetFromURL(""))
	assert.Equal(t, "", targetFromURL("://bad"))
}

func TestNormalizeTargets(t *testing.T) {
	got := normalizeTargets([]string{" 1.1.1.1 ", "example.com:80", "1.1.1.1:443", ""})
	assert.Equal(t, []string{"1.1.1.1:443", "example.com:80"}, got)
}

func TestTcpConnectLatency(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	ms, err := tcpConnectLatency(ln.Addr().String(), 2*time.Second)
	require.NoError(t, err)
	assert.Greater(t, ms, 0.0)
	assert.Less(t, ms, 2000.0)

	_, err = tcpConnectLatency("127.0.0.1:1", 200*time.Millisecond)
	assert.Error(t, err)
}

func TestLatencyManagerProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hostPort := srv.Listener.Addr().String()
	lm := &latencyManager{
		targets: []string{hostPort, "127.0.0.1:1"},
		timeout: 500 * time.Millisecond,
	}

	avg, results := lm.probe()
	require.Greater(t, avg, 0.0)
	require.Contains(t, results, hostPort)
	assert.NotContains(t, results, "127.0.0.1:1")
	assert.Equal(t, results[hostPort], avg)
}

func TestLatencyManagerProbeAllFail(t *testing.T) {
	lm := &latencyManager{
		targets: []string{"127.0.0.1:1"},
		timeout: 100 * time.Millisecond,
	}
	avg, results := lm.probe()
	assert.Equal(t, 0.0, avg)
	assert.Nil(t, results)
}
