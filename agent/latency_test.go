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

func TestParseNamedTargets(t *testing.T) {
	got := parseNamedTargets("电信广东=gd-ct-v4.ip.zstaticcdn.com:80,移动广东=gd-cm-v4.ip.zstaticcdn.com:80")
	require.Len(t, got, 2)
	assert.Equal(t, "电信广东", got[0].Name)
	assert.Equal(t, "gd-ct-v4.ip.zstaticcdn.com:80", got[0].Addr)
	assert.Equal(t, "移动广东", got[1].Name)

	// multi-line
	got = parseNamedTargets("电信广东=gd-ct-v4.ip.zstaticcdn.com:80\n联通广东=gd-cu-v4.ip.zstaticcdn.com:80\n")
	require.Len(t, got, 2)
	assert.Equal(t, "联通广东", got[1].Name)

	// bare host defaults name to address
	got = parseNamedTargets("1.1.1.1:443")
	require.Len(t, got, 1)
	assert.Equal(t, "1.1.1.1:443", got[0].Name)
	assert.Equal(t, "1.1.1.1:443", got[0].Addr)
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
		targets: []namedTarget{
			{Name: "本机测试", Addr: hostPort},
			{Name: "失败点", Addr: "127.0.0.1:1"},
		},
		timeout: 500 * time.Millisecond,
	}

	avg, results := lm.probe()
	require.Greater(t, avg, 0.0)
	require.Contains(t, results, "本机测试")
	assert.NotContains(t, results, "失败点")
	assert.Equal(t, results["本机测试"], avg)
}

func TestLatencyManagerProbeAllFail(t *testing.T) {
	lm := &latencyManager{
		targets: []namedTarget{{Name: "x", Addr: "127.0.0.1:1"}},
		timeout: 100 * time.Millisecond,
	}
	avg, results := lm.probe()
	assert.Equal(t, 0.0, avg)
	assert.Nil(t, results)
}

func TestApplyHubTargetsNamed(t *testing.T) {
	lm := &latencyManager{
		envTargets: []namedTarget{{Name: "1.1.1.1:443", Addr: "1.1.1.1:443"}},
		targets:    []namedTarget{{Name: "1.1.1.1:443", Addr: "1.1.1.1:443"}},
		timeout:    time.Second,
	}
	changed := lm.applyHubTargets("电信广东=gd-ct-v4.ip.zstaticcdn.com:80\n移动广东=gd-cm-v4.ip.zstaticcdn.com:80")
	assert.True(t, changed)
	assert.True(t, lm.hubOverride)
	require.Len(t, lm.targets, 2)
	assert.Equal(t, "电信广东", lm.targets[0].Name)

	// empty resets to env
	changed = lm.applyHubTargets("")
	assert.True(t, changed)
	assert.False(t, lm.hubOverride)
	assert.Equal(t, "1.1.1.1:443", lm.targets[0].Name)
}
