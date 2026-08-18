//go:build testing

package systems

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// newTestSSHClient returns a live *ssh.Client backed by an in-process SSH
// server listening on loopback TCP, so tests can exercise real
// NewSession()/Close() concurrency instead of a nil stand-in. (net.Pipe is
// unsuitable here: the SSH version exchange writes from both ends before
// either reads, which deadlocks on its unbuffered synchronous conn.)
func newTestSSHClient(t *testing.T) *ssh.Client {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)

	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		sconn, chans, reqs, err := ssh.NewServerConn(conn, serverConfig)
		if err != nil {
			return
		}
		defer sconn.Close()
		go ssh.DiscardRequests(reqs)
		for newChan := range chans {
			ch, chReqs, err := newChan.Accept()
			if err != nil {
				continue
			}
			go ssh.DiscardRequests(chReqs)
			go ch.Close()
		}
	}()

	clientConfig := &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", ln.Addr().String(), clientConfig)
	require.NoError(t, err)
	return client
}

// TestSystemClientRace guards against the TOCTOU race from issue #2157: a
// concurrent closeSSHConnection nil-ing sys.client while
// createSessionWithTimeout is reading it must never panic, and (run with
// -race) must never be reported as a data race.
func TestSystemClientRace(t *testing.T) {
	for i := 0; i < 20; i++ {
		sys := &System{ctx: context.Background()}
		sys.setClient(newTestSSHClient(t))

		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, _ = sys.createSessionWithTimeout(2 * time.Second)
		}()
		go func() {
			defer wg.Done()
			sys.closeSSHConnection()
		}()
		go func() {
			defer wg.Done()
			_ = sys.getClient()
		}()
		wg.Wait()
	}
}

// TestSystemClientRace_MultipleReaders further stresses concurrent readers
// of sys.client racing against a close, mirroring runSSHOperation's
// nil-check followed by createSessionWithTimeout's own read.
func TestSystemClientRace_MultipleReaders(t *testing.T) {
	sys := &System{ctx: context.Background()}
	sys.setClient(newTestSSHClient(t))

	var wg sync.WaitGroup
	stop := make(chan struct{})
	defer close(stop)

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if sys.getClient() == nil {
					return
				}
				_, _ = sys.createSessionWithTimeout(100 * time.Millisecond)
			}
		}()
	}

	sys.closeSSHConnection()
	wg.Wait()
}
