package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Adapted from the audit's loopback deadline reproduction. Only the first
// connection stalls; later connections provide a real reconnection control.
func newSSHTestTransport(t *testing.T, stage string) (*SSHTransport, <-chan struct{}, <-chan struct{}) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	config := &ssh.ServerConfig{NoClientAuth: true, ServerVersion: "SSH-2.0-beszel_0.20.0"}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	transport := NewSSHTransport(SSHTransportConfig{
		Host: host, Port: port, Timeout: 2 * time.Second,
		Config: &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: time.Second},
	})
	reached, closed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var connections atomic.Int32
	var mu sync.Mutex
	conns := map[net.Conn]bool{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns[conn] = true
			mu.Unlock()
			first := connections.Add(1) == 1
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				if first {
					defer close(closed)
				}
				if first && stage == "handshake" {
					close(reached)
					_, _ = io.Copy(io.Discard, conn)
					return
				}
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				disconnected := make(chan struct{})
				go func() { _ = server.Wait(); close(disconnected) }()
				for channel := range channels {
					wg.Add(1)
					go func() {
						defer wg.Done()
						stall := func(at string) bool {
							if !first || stage != at {
								return false
							}
							once.Do(func() { close(reached) })
							<-disconnected
							return true
						}
						if stall("session") {
							return
						}
						ch, requests, err := channel.Accept()
						if err != nil {
							return
						}
						defer ch.Close()
						for request := range requests {
							if request.Type != "shell" {
								_ = request.Reply(false, nil)
								continue
							}
							if stall("shell") {
								return
							}
							_ = request.Reply(true, nil)
							if stall("write") {
								return
							}
							var req common.HubRequest[cbor.RawMessage]
							if cbor.NewDecoder(ch).Decode(&req) != nil || stall("response") {
								return
							}
							if stage == "slow-response" {
								select {
								case <-time.After(250 * time.Millisecond):
								case <-disconnected:
									return
								}
							}
							data, _ := cbor.Marshal("control response")
							if cbor.NewEncoder(ch).Encode(common.AgentResponse{Data: data}) != nil || stall("exit") {
								return
							}
							_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							return
						}
					}()
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		transport.Close()
		mu.Lock()
		for conn := range conns {
			conn.Close()
		}
		mu.Unlock()
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("SSH server did not stop")
		}
	})
	return transport, reached, closed
}

func TestSSHRequestCancellation(t *testing.T) {
	for _, stage := range []string{"handshake", "session", "shell", "write", "response", "exit"} {
		for _, cancellation := range []string{"deadline", "cancel"} {
			t.Run(stage+"/"+cancellation, func(t *testing.T) {
				transport, reached, closed := newSSHTestTransport(t, stage)
				var req any
				if stage == "write" {
					// Larger than the SSH receive window, so an unread write blocks.
					req = strings.Repeat("x", 4<<20)
				}
				ctx, cancel := context.WithCancel(t.Context())
				if cancellation == "deadline" {
					cancel()
					// Handshake has its own deadline case. Complete it before
					// timing the later SSH phases on slower hosts.
					if stage != "handshake" {
						setupCtx, stopSetup := context.WithTimeout(t.Context(), 2*time.Second)
						_, err := transport.connect(setupCtx)
						stopSetup()
						require.NoError(t, err)
					}
					ctx, cancel = context.WithTimeout(t.Context(), 250*time.Millisecond)
				}
				defer cancel()
				done := make(chan error, 1)
				result := "unchanged"
				go func() { done <- transport.RequestWithRetry(ctx, common.GetContainerLogs, req, &result, 1) }()
				select {
				case <-reached:
				case <-time.After(2 * time.Second):
					t.Fatal("request did not reach stalled phase")
				}
				want := context.DeadlineExceeded
				if cancellation == "cancel" {
					want = context.Canceled
					cancel()
				}
				select {
				case err := <-done:
					require.ErrorIs(t, err, want)
				case <-time.After(2 * time.Second):
					t.Fatal("request ignored cancellation")
				}
				require.Equal(t, "unchanged", result)
				require.False(t, transport.IsConnected())
				select {
				case <-closed:
				case <-time.After(time.Second):
					t.Fatal("cancelled request left its connection open")
				}
				require.NoError(t, transport.Request(t.Context(), common.GetContainerLogs, nil, &result))
				require.Equal(t, "control response", result)
			})
		}
	}
}

func TestSSHSessionTimeout(t *testing.T) {
	transport, _, _ := newSSHTestTransport(t, "session")
	transport.timeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var result string
	require.ErrorIs(t, transport.Request(ctx, common.GetContainerLogs, nil, &result), context.DeadlineExceeded)
	require.NoError(t, ctx.Err(), "the session timeout must fire before the caller's deadline")
	require.False(t, transport.IsConnected())
}

func TestSSHRequestAlreadyCancelled(t *testing.T) {
	transport := NewSSHTransport(SSHTransportConfig{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var result string
	require.ErrorIs(t, transport.Request(ctx, common.GetContainerLogs, nil, &result), context.Canceled)
}

func TestSSHSlowResponse(t *testing.T) {
	transport, _, _ := newSSHTestTransport(t, "slow-response")
	transport.timeout = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var result string
	require.NoError(t, transport.Request(ctx, common.GetContainerLogs, nil, &result))
	require.Equal(t, "control response", result, "session timeout must not shorten the request deadline")
}

func TestSSHConcurrentRequests(t *testing.T) {
	transport, _, _ := newSSHTestTransport(t, "")
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			var result string
			if err := transport.Request(ctx, common.GetContainerLogs, nil, &result); err != nil {
				t.Error(err)
			} else if result != "control response" {
				t.Errorf("unexpected response %q", result)
			}
		})
	}
	wg.Wait()
	client := transport.GetClient()
	require.NotNil(t, client)
	ctx, cancel := context.WithCancel(t.Context())
	var result string
	require.NoError(t, transport.Request(ctx, common.GetContainerLogs, nil, &result))
	cancel()
	// Cancelling a completed request must not close the reused connection.
	require.NoError(t, transport.Request(t.Context(), common.GetContainerLogs, nil, &result))
	require.Same(t, client, transport.GetClient())
	// The existing retry contract still replaces an unusable connection.
	require.NoError(t, client.Close())
	require.NoError(t, transport.RequestWithRetry(t.Context(), common.GetContainerLogs, nil, &result, 1))
	require.NotSame(t, client, transport.GetClient())
}

func TestSSHCancelledSharedConnection(t *testing.T) {
	transport, reached, _ := newSSHTestTransport(t, "response")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	client, err := transport.connect(ctx)
	require.NoError(t, err)
	// Keep another session on the shared connection waiting for an exit.
	session, err := client.NewSession()
	require.NoError(t, err)
	require.NoError(t, session.Shell())
	waiting := make(chan error, 1)
	go func() { waiting <- session.Wait() }()
	requestCtx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()
	done := make(chan error, 1)
	var result string
	go func() { done <- transport.Request(requestCtx, common.GetContainerLogs, nil, &result) }()
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("request did not reach the response read")
	}
	cancelRequest()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-ctx.Done():
		t.Fatal("request ignored cancellation")
	}
	select {
	case err := <-waiting:
		require.Error(t, err, "closing the shared client must release other sessions")
	case <-ctx.Done():
		t.Fatal("concurrent session remained blocked")
	}
	require.NoError(t, transport.Request(ctx, common.GetContainerLogs, nil, &result))
	replacement := transport.GetClient()
	require.NotSame(t, client, replacement)
	// Late cleanup of the old connection must not discard its replacement.
	transport.closeClient(client)
	require.Same(t, replacement, transport.GetClient())
	require.NoError(t, transport.Request(ctx, common.GetContainerLogs, nil, &result))
}
