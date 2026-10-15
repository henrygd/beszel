//go:build testing

package agent

import (
	"crypto/ed25519"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/common"

	"github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

func TestSSHServerConfigConcurrentConnections(t *testing.T) {
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(key)
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	const connections = 2
	configs := make(chan *gossh.ServerConfig, connections)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	server := &ssh.Server{
		HostSigners: []ssh.Signer{signer},
		ServerConfigCallback: func(ctx ssh.Context) *gossh.ServerConfig {
			config := newSSHServerConfig(ctx)
			configs <- config
			// Both connections must obtain their configuration before either
			// lets gliderlabs add host keys and connection-specific callbacks.
			<-release
			return config
		},
		PublicKeyHandler: func(_ ssh.Context, key ssh.PublicKey) bool {
			return ssh.KeysEqual(key, signer.PublicKey())
		},
		Handler: func(session ssh.Session) { _ = session.Exit(0) },
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() {
		unblock()
		_ = listener.Close()
		_ = server.Close()
		select {
		case <-served:
		case <-time.After(5 * time.Second):
			t.Error("SSH test server did not stop")
		}
	})

	results := make(chan error, connections)
	for range connections {
		go func() {
			conn, err := net.DialTimeout("tcp", listener.Addr().String(), 5*time.Second)
			if err != nil {
				results <- err
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			client, _, _, err := gossh.NewClientConn(conn, listener.Addr().String(), &gossh.ClientConfig{
				User:            "test",
				Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
				HostKeyCallback: gossh.FixedHostKey(signer.PublicKey()),
			})
			if err == nil {
				err = client.Close()
			}
			results <- err
		}()
	}

	var first *gossh.ServerConfig
	for range connections {
		select {
		case config := <-configs:
			assert.Equal(t, fmt.Sprintf("SSH-2.0-%s_%s", beszel.AppName, beszel.Version), config.ServerVersion)
			assert.Equal(t, common.DefaultKeyExchanges, config.KeyExchanges)
			assert.Equal(t, common.DefaultMACs, config.MACs)
			assert.Equal(t, common.DefaultCiphers, config.Ciphers)
			if first == nil {
				first = config
			} else {
				assert.NotSame(t, first, config, "SSH connections must not share mutable configuration")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("SSH connections did not reach their config callbacks")
		}
	}
	unblock()
	for range connections {
		select {
		case err := <-results:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("SSH handshake did not finish")
		}
	}
}
