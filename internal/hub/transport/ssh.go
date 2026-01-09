package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"golang.org/x/crypto/ssh"
)

// SSHTransport implements Transport over SSH connections.
type SSHTransport struct {
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
	t.client = client
}

// SetAgentVersion sets the agent version (extracted from SSH handshake).
func (t *SSHTransport) SetAgentVersion(version semver.Version) {
	t.agentVersion = version
}

// GetClient returns the current SSH client (for connection management).
func (t *SSHTransport) GetClient() *ssh.Client {
	return t.client
}

// GetAgentVersion returns the agent version.
func (t *SSHTransport) GetAgentVersion() semver.Version {
	return t.agentVersion
}

// Request sends a request to the agent via SSH and unmarshals the response.
func (t *SSHTransport) Request(ctx context.Context, action common.WebSocketAction, req any, dest any) error {
	if t.client == nil {
		if err := t.connect(); err != nil {
			return err
		}
	}

	session, err := t.createSessionWithTimeout(ctx)
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
	return t.client != nil
}

// Close terminates the SSH connection.
func (t *SSHTransport) Close() {
	if t.client != nil {
		t.client.Close()
		t.client = nil
	}
}

// connect establishes a new SSH connection.
func (t *SSHTransport) connect() error {
	if t.config == nil {
		return errors.New("SSH config not set")
	}

	network := "tcp"
	host := t.host
	if strings.HasPrefix(host, "/") {
		network = "unix"
	} else {
		host = net.JoinHostPort(host, t.port)
	}

	client, err := ssh.Dial(network, host, t.config)
	if err != nil {
		return err
	}
	t.client = client

	// Extract agent version from server version string
	t.agentVersion, _ = extractAgentVersion(string(client.Conn.ServerVersion()))
	return nil
}

// createSessionWithTimeout creates a new SSH session with a timeout.
func (t *SSHTransport) createSessionWithTimeout(ctx context.Context) (*ssh.Session, error) {
	if t.client == nil {
		return nil, errors.New("client not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	sessionChan := make(chan *ssh.Session, 1)
	errChan := make(chan error, 1)

	go func() {
		session, err := t.client.NewSession()
		if err != nil {
			errChan <- err
		} else {
			sessionChan <- session
		}
	}()

	select {
	case session := <-sessionChan:
		return session, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, errors.New("timeout creating session")
	}
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
			t.Close()
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
