package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"golang.org/x/crypto/ssh"
)

// sshKeepAliveInterval is the TCP keep-alive idle interval for SSH connections
// to agents. Enabling OS-level keep-alives lets the hub eventually detect a
// dead peer on an otherwise idle connection instead of trusting it forever.
// This is a backstop for genuine network death; an application-level wedge
// (agent process hung while its kernel keeps ACKing) is caught by per-operation
// timeouts instead (see issue #2041).
const sshKeepAliveInterval = 30 * time.Second

// sshHandshakeTimeout bounds the SSH handshake after the TCP connection is
// established. ssh.ClientConfig.Timeout only covers the TCP connect, so a peer
// that accepts the connection but never sends an SSH banner would otherwise
// block the caller forever (GHSA-h9jh-29rh-w464).
var sshHandshakeTimeout = 10 * time.Second

// SSHTransport implements Transport over SSH connections. It owns the single
// SSH connection to an agent, which is shared by all requests and sessions.
type SSHTransport struct {
	mu        sync.Mutex
	client    *ssh.Client
	config    *ssh.ClientConfig
	host      string
	port      string
	timeout   time.Duration
	onConnect func(agentVersion semver.Version)
}

// SSHTransportConfig holds configuration for creating an SSH transport.
type SSHTransportConfig struct {
	Host    string
	Port    string
	Config  *ssh.ClientConfig
	Timeout time.Duration
	// OnConnect is called when a new connection is established, before it is
	// available to other callers, with the agent version from its SSH server
	// version string. It runs under the transport lock and must not call back
	// into the transport.
	OnConnect func(agentVersion semver.Version)
}

// NewSSHTransport creates a new SSH transport with the given configuration.
func NewSSHTransport(cfg SSHTransportConfig) *SSHTransport {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 4 * time.Second
	}
	return &SSHTransport{
		config:    cfg.Config,
		host:      cfg.Host,
		port:      cfg.Port,
		timeout:   timeout,
		onConnect: cfg.OnConnect,
	}
}

// GetClient returns the current SSH client, or nil if not connected.
func (t *SSHTransport) GetClient() *ssh.Client {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.client
}

// Request sends a request to the agent via SSH and unmarshals the response.
func (t *SSHTransport) Request(ctx context.Context, action common.WebSocketAction, req any, dest any) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := t.Connect(ctx)
	if err != nil {
		return err
	}

	// Closing only the session still depends on the peer processing SSH packets.
	// Close the captured connection to release every blocked read/write, including
	// concurrent sessions; subsequent requests can reconnect.
	stop := closeOnCancellation(ctx, func() { t.CloseClient(client) })
	defer func() {
		stop()
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		if isConnectionError(err) {
			t.CloseClient(client)
		}
	}()

	session, err := t.NewSession(ctx, client)
	if err != nil {
		return err
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	if err := session.Shell(); err != nil {
		return err
	}

	// Send request
	hubReq := common.HubRequest[any]{Action: action, Data: req}
	if err := cbor.NewEncoder(stdin).Encode(hubReq); err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	stdin.Close()

	// Read response
	var resp common.AgentResponse
	if err := cbor.NewDecoder(stdout).Decode(&resp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.Error != "" {
		return errors.New(resp.Error)
	}

	if err := session.Wait(); err != nil {
		return err
	}

	return UnmarshalResponse(resp, action, dest)
}

// IsConnected returns true if the SSH connection is active.
func (t *SSHTransport) IsConnected() bool {
	return t.GetClient() != nil
}

// Close terminates the SSH connection.
func (t *SSHTransport) Close() {
	t.CloseClient(t.GetClient())
}

// CloseClient closes client and clears it if it is still the current
// connection, so a late close never discards a replacement connection.
func (t *SSHTransport) CloseClient(client *ssh.Client) {
	t.mu.Lock()
	if t.client == client {
		t.client = nil
	}
	t.mu.Unlock()
	if client != nil {
		client.Close()
	}
}

// closeOnCancellation stops I/O when ctx is cancelled. The returned function
// waits for any in-progress close so it cannot outlive the operation.
func closeOnCancellation(ctx context.Context, closeConn func()) func() {
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		closeConn()
		close(done)
	})
	return func() {
		if !stop() {
			<-done
		}
	}
}

// Connect returns the current client or establishes a new cancellable SSH
// connection, calling OnConnect when a new connection is stored.
func (t *SSHTransport) Connect(ctx context.Context) (*ssh.Client, error) {
	if client := t.GetClient(); client != nil {
		return client, nil
	}
	if t.config == nil {
		return nil, errors.New("SSH config not set")
	}

	network := "tcp"
	host := t.host
	if strings.HasPrefix(host, "/") {
		network = "unix"
	} else {
		host = net.JoinHostPort(host, t.port)
	}

	dialer := net.Dialer{Timeout: t.config.Timeout, KeepAlive: sshKeepAliveInterval}
	conn, err := dialer.DialContext(ctx, network, host)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(sshHandshakeTimeout))
	stop := closeOnCancellation(ctx, func() { conn.Close() })
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, host, t.config)
	stop()
	if ctx.Err() != nil {
		conn.Close()
		return nil, ctx.Err()
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	// clear the handshake deadline so it doesn't apply to the long-lived connection
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(sshConn, chans, reqs)

	t.mu.Lock()
	if existing := t.client; existing != nil {
		t.mu.Unlock()
		client.Close()
		return existing, nil
	}
	// Initialize per-connection state (e.g. the agent version, which selects the
	// protocol) before other callers can reuse the client.
	if t.onConnect != nil {
		agentVersion, _ := extractAgentVersion(string(client.Conn.ServerVersion()))
		t.onConnect(agentVersion)
	}
	t.client = client
	t.mu.Unlock()
	return client, nil
}

// NewSession opens a session on client, bounded by the transport timeout
// independently of ctx. The connection is closed if session creation stalls.
func (t *SSHTransport) NewSession(ctx context.Context, client *ssh.Client) (*ssh.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	stop := closeOnCancellation(ctx, func() { t.CloseClient(client) })
	session, err := client.NewSession()
	stop()
	if ctx.Err() != nil {
		if session != nil {
			session.Close()
		}
		return nil, ctx.Err()
	}
	return session, err
}

// extractAgentVersion extracts the beszel version from SSH server version string.
func extractAgentVersion(versionString string) (semver.Version, error) {
	_, after, _ := strings.Cut(versionString, "_")
	return semver.Parse(after)
}

// RequestWithRetry sends a request with automatic retry on connection failures.
func (t *SSHTransport) RequestWithRetry(ctx context.Context, action common.WebSocketAction, req any, dest any, retries int) error {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		err := t.Request(ctx, action, req, dest)
		if err == nil {
			return nil
		}
		lastErr = err

		// Check if it's a connection error that warrants a retry
		if isConnectionError(err) && attempt < retries {
			continue
		}
		return err
	}
	return lastErr
}

// isConnectionError checks if an error indicates a connection problem.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "closed") ||
		errors.Is(err, io.EOF)
}
