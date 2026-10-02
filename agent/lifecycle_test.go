//go:build testing

package agent

import (
	"context"
	"crypto/ed25519"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestAgentCancellationReleasesListener(t *testing.T) {
	t.Setenv("BESZEL_AGENT_HUB_URL", "")
	opts := createTestServerOptions(t)
	for range 2 {
		a := createTestAgent(t)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- a.start(ctx, opts) }()
		require.Eventually(t, func() bool {
			conn, err := net.DialTimeout("tcp", opts.Addr, 20*time.Millisecond)
			if err != nil {
				return false
			}
			conn.Close()
			return true
		}, 2*time.Second, 10*time.Millisecond)
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("agent did not stop after cancellation")
		}
		listener, err := net.Listen("tcp", opts.Addr)
		require.NoError(t, err, "stopped agent must release its listening port")
		require.NoError(t, listener.Close())
	}
}

func TestAgentCancellationJoinsReconnect(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer hub.Close()
	t.Setenv("BESZEL_AGENT_HUB_URL", hub.URL)
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")
	a := createTestAgent(t)
	opts := createTestServerOptions(t)
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	opts.Keys = []ssh.PublicKey{signer.PublicKey()}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.start(ctx, opts) }()
	var client *ssh.Client
	require.Eventually(t, func() bool {
		client, err = ssh.Dial("tcp", opts.Addr, &ssh.ClientConfig{
			User: "test", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second,
		})
		return err == nil
	}, 2*time.Second, 10*time.Millisecond)
	defer client.Close()
	session, err := client.NewSession()
	require.NoError(t, err)
	require.NoError(t, session.Shell())
	cm := a.connectionManager
	require.Eventually(t, func() bool { return cm.getState() == SSHConnected }, time.Second, time.Millisecond)
	require.NoError(t, client.Close())
	require.Eventually(t, cm.isConnectingNow, time.Second, time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not join its pending reconnect")
	}
	require.False(t, cm.isConnectingNow())
	listener, err := net.Listen("tcp", opts.Addr)
	require.NoError(t, err)
	require.NoError(t, listener.Close())
}

func TestAgentCancellationDuringHandshake(t *testing.T) {
	entered := make(chan struct{})
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer hub.Close()
	t.Setenv("BESZEL_AGENT_HUB_URL", hub.URL)
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")
	a := createTestAgent(t)
	opts := createTestServerOptions(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.start(ctx, opts) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not reach the HTTP upgrade")
	}
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Error("agent did not cancel its pending handshake")
	}
}
