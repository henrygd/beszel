package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// newDialTestTransport returns a transport that dials ln with config.
func newDialTestTransport(t *testing.T, ln net.Listener, config *ssh.ClientConfig) *SSHTransport {
	t.Helper()
	host, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	return NewSSHTransport(SSHTransportConfig{Host: host, Port: port, Config: config})
}

// closedConn stands in for a connection whose peer has gone away: opening a
// channel fails rather than succeeding, which is what NewSession does on a
// client that has already been closed.
type closedConn struct{ ssh.Conn }

func (closedConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, errors.New("use of closed network connection")
}

func (closedConn) Close() error { return nil }

// TestNewSessionDuringClose covers issue #2157: a background request creates a
// session while the updater can be tearing the same connection down, so session
// creation must use the captured client rather than the cleared field.
func TestNewSessionDuringClose(t *testing.T) {
	for range 500 {
		transport := NewSSHTransport(SSHTransportConfig{})
		transport.client = &ssh.Client{Conn: closedConn{}}

		var wg sync.WaitGroup
		wg.Go(func() {
			client, err := transport.Connect(t.Context())
			if err != nil {
				return // already closed; no config to re-dial
			}
			session, err := transport.NewSession(t.Context(), client)
			assert.Nil(t, session)
			assert.Error(t, err, "a closed connection must surface an error, not a session")
		})
		wg.Go(transport.Close)
		wg.Wait()
	}
}

// TestConnectHandshakeTimeout covers a peer that accepts the TCP connection but
// never sends an SSH banner. Without a handshake deadline the dial blocks the
// caller forever (GHSA-h9jh-29rh-w464).
func TestConnectHandshakeTimeout(t *testing.T) {
	prev := sshHandshakeTimeout
	sshHandshakeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { sshHandshakeTimeout = prev })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	config := &ssh.ClientConfig{
		User:            "u",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         4 * time.Second,
	}

	transport := newDialTestTransport(t, ln, config)
	done := make(chan error, 1)
	go func() {
		_, err := transport.Connect(context.Background())
		transport.Close()
		done <- err
	}()

	select {
	case err := <-done:
		assert.Error(t, err, "a silent peer must fail the handshake")
	case <-time.After(5 * time.Second):
		t.Fatal("dial blocked on a peer that never sends an SSH banner")
	}

	// the hub must close its side of the connection
	conn := <-accepted
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(make([]byte, 256))
	for err == nil {
		_, err = conn.Read(make([]byte, 256))
	}
	var netErr net.Error
	assert.False(t, errors.As(err, &netErr) && netErr.Timeout(), "hub should close the connection, got %v", err)
}

// TestConnectClearsHandshakeDeadline ensures the handshake deadline does not
// carry over to the established connection, which is reused for many updates.
func TestConnectClearsHandshakeDeadline(t *testing.T) {
	prev := sshHandshakeTimeout
	sshHandshakeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { sshHandshakeTimeout = prev })

	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	require.NoError(t, err)
	_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	clientSigner, err := ssh.NewSignerFromKey(clientPriv)
	require.NoError(t, err)

	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) { return nil, nil },
	}
	serverConfig.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, chans, reqs, err := ssh.NewServerConn(conn, serverConfig)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(reqs)
		for newChan := range chans {
			ch, chReqs, err := newChan.Accept()
			if err != nil {
				continue
			}
			go ssh.DiscardRequests(chReqs)
			ch.Close()
		}
	}()

	config := &ssh.ClientConfig{
		User:            "u",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(clientSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         4 * time.Second,
	}
	transport := newDialTestTransport(t, ln, config)
	client, err := transport.Connect(context.Background())
	require.NoError(t, err)
	defer transport.Close()

	// wait past the handshake deadline; the connection must still be usable
	time.Sleep(3 * sshHandshakeTimeout)
	session, err := client.NewSession()
	require.NoError(t, err, "connection should outlive the handshake deadline")
	session.Close()
}
