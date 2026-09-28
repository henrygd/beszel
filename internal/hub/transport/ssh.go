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

// SSHTransport implements Transport over SSH connections.
type SSHTransport struct {
	mu           sync.Mutex
	client       *ssh.Client
	config       *ssh.ClientConfig
	host         string
	port         string
	agentVersion semver.Version
	timeout      time.Duration
}

// SSHTransportConfig holds configuration for creating an SSH transport.
type SSHTransportConfig struct {
	Host         string
	Port         string
	Config       *ssh.ClientConfig
	AgentVersion semver.Version
	Timeout      time.Duration
}

// NewSSHTransport creates a new SSH transport with the given configuration.
func NewSSHTransport(cfg SSHTransportConfig) *SSHTransport {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 4 * time.Second
	}
	return &SSHTransport{
		config:       cfg.Config,
		host:         cfg.Host,
		port:         cfg.Port,
		agentVersion: cfg.AgentVersion,
		timeout:      timeout,
	}
}

// SetClient sets the SSH client for reuse across requests.
func (t *SSHTransport) SetClient(client *ssh.Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.client = client
}

// SetAgentVersion sets the agent version (extracted from SSH handshake).
func (t *SSHTransport) SetAgentVersion(version semver.Version) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.agentVersion = version
}

// GetClient returns the current SSH client (for connection management).
func (t *SSHTransport) GetClient() *ssh.Client {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.client
}

// GetAgentVersion returns the agent version.
func (t *SSHTransport) GetAgentVersion() semver.Version {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.agentVersion
}

// Request sends a request to the agent via SSH and unmarshals the response.
func (t *SSHTransport) Request(ctx context.Context, action common.WebSocketAction, req any, dest any) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := t.connect(ctx)
	if err != nil {
		return err
	}

	// Closing only the session still depends on the peer processing SSH packets.
	// Close the captured connection to release every blocked read/write, including
	// concurrent sessions; subsequent requests can reconnect.
	stop := closeOnCancellation(ctx, func() { t.closeClient(client) })
	defer func() {
		stop()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if isConnectionError(err) {
			t.closeClient(client)
		}
	}()

	session, err := t.createSessionWithTimeout(ctx, client)
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
	t.closeClient(t.GetClient())
}

// closeClient removes only the connection owned by the completed request.
func (t *SSHTransport) closeClient(client *ssh.Client) {
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

// connect reuses the current client or establishes a cancellable SSH connection.
func (t *SSHTransport) connect(ctx context.Context) (*ssh.Client, error) {
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

	dialer := net.Dialer{Timeout: t.config.Timeout}
	conn, err := dialer.DialContext(ctx, network, host)
	if err != nil {
		return nil, err
	}
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
	client := ssh.NewClient(sshConn, chans, reqs)

	t.mu.Lock()
	if existing := t.client; existing != nil {
		t.mu.Unlock()
		client.Close()
		return existing, nil
	}
	t.client = client
	t.agentVersion, _ = extractAgentVersion(string(client.Conn.ServerVersion()))
	t.mu.Unlock()
	return client, nil
}

// createSessionWithTimeout bounds session creation independently of the request.
func (t *SSHTransport) createSessionWithTimeout(ctx context.Context, client *ssh.Client) (*ssh.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	stop := closeOnCancellation(ctx, func() { t.closeClient(client) })
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
