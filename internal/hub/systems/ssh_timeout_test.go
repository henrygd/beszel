//go:build testing

package systems

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// TestRunWithTimeout covers the guard added for issue #2041: the per-system SSH
// data exchange must never block the updater indefinitely on a dead connection.
func TestRunWithTimeout(t *testing.T) {
	t.Run("returns the operation result when it completes before the timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			wantErr := errors.New("boom")
			onTimeoutCalled := false

			retry, err := runWithTimeout(10*time.Second, func() (bool, error) {
				return true, wantErr
			}, func() { onTimeoutCalled = true })

			assert.True(t, retry, "should return the operation's retry value")
			assert.Equal(t, wantErr, err, "should return the operation's error")
			assert.False(t, onTimeoutCalled, "onTimeout must not fire when the op completes")
		})
	})

	t.Run("times out and tears down the connection when the op blocks", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// unblock simulates a half-open connection: the op is stuck reading a
			// response that never arrives until the connection is torn down.
			unblock := make(chan struct{})
			onTimeoutCalled := false
			start := time.Now()

			retry, err := runWithTimeout(5*time.Second, func() (bool, error) {
				<-unblock
				return false, nil
			}, func() {
				onTimeoutCalled = true
				close(unblock) // tearing down the connection releases the blocked read
			})

			assert.Equal(t, 5*time.Second, time.Since(start), "should return exactly at the timeout")
			assert.True(t, retry, "a timeout should be retryable so the next tick re-dials")
			assert.Error(t, err, "a timeout must surface an error so the system is set down")
			assert.True(t, onTimeoutCalled, "onTimeout must fire so the dead connection is closed")

			synctest.Wait() // ensure the released op goroutine exits cleanly
		})
	})
}

// closedConn stands in for a connection whose peer has gone away: opening a
// channel fails rather than succeeding, which is what NewSession does on a
// client that closeSSHConnection has already closed.
type closedConn struct{ ssh.Conn }

func (closedConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, errors.New("use of closed network connection")
}

func (closedConn) Close() error { return nil }

// TestCreateSessionDuringClose covers issue #2157: the background SMART fetch
// creates a session while the updater can be tearing the same connection down,
// so session creation must not read the client field after it is cleared.
func TestCreateSessionDuringClose(t *testing.T) {
	for range 500 {
		sys := &System{ctx: t.Context()}
		sys.client.Store(&ssh.Client{Conn: closedConn{}})

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			session, err := sys.createSessionWithTimeout(time.Second)
			assert.Nil(t, session)
			assert.Error(t, err, "a closed connection must surface an error, not a session")
		}()
		go func() {
			defer wg.Done()
			sys.closeSSHConnection()
		}()
		wg.Wait()
	}
}

// TestDialSSHHandshakeTimeout covers a peer that accepts the TCP connection but
// never sends an SSH banner. Without a handshake deadline the dial blocks the
// updater forever (GHSA-h9jh-29rh-w464).
func TestDialSSHHandshakeTimeout(t *testing.T) {
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

	done := make(chan error, 1)
	go func() {
		client, err := dialSSHWithKeepAlive("tcp", ln.Addr().String(), config)
		if client != nil {
			client.Close()
		}
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

// TestDialSSHClearsHandshakeDeadline ensures the handshake deadline does not
// carry over to the established connection, which is reused for many updates.
func TestDialSSHClearsHandshakeDeadline(t *testing.T) {
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
	client, err := dialSSHWithKeepAlive("tcp", ln.Addr().String(), config)
	require.NoError(t, err)
	defer client.Close()

	// wait past the handshake deadline; the connection must still be usable
	time.Sleep(3 * sshHandshakeTimeout)
	session, err := client.NewSession()
	require.NoError(t, err, "connection should outlive the handshake deadline")
	session.Close()
}
