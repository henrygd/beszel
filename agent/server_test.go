//go:build testing

package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/gliderlabs/ssh"
	"github.com/lxzan/gws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

func TestStartServer(t *testing.T) {
	// Generate a test key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(privKey)
	require.NoError(t, err)
	sshPubKey, err := gossh.NewPublicKey(pubKey)
	require.NoError(t, err)

	// Generate a different key pair for bad key test
	badPubKey, badPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	badSigner, err := gossh.NewSignerFromKey(badPrivKey)
	require.NoError(t, err)
	sshBadPubKey, err := gossh.NewPublicKey(badPubKey)
	require.NoError(t, err)

	socketFile := filepath.Join(t.TempDir(), "beszel-test.sock")

	tests := []struct {
		name        string
		config      ServerOptions
		wantErr     bool
		errContains string
		setup       func() error
		cleanup     func() error
	}{
		{
			name: "tcp port only",
			config: ServerOptions{
				Network: "tcp",
				Addr:    ":45987",
				Keys:    []gossh.PublicKey{sshPubKey},
			},
		},
		{
			name: "tcp with ipv4",
			config: ServerOptions{
				Network: "tcp4",
				Addr:    "127.0.0.1:45988",
				Keys:    []gossh.PublicKey{sshPubKey},
			},
		},
		{
			name: "tcp with ipv6",
			config: ServerOptions{
				Network: "tcp6",
				Addr:    "[::1]:45989",
				Keys:    []gossh.PublicKey{sshPubKey},
			},
		},
		{
			name: "unix socket",
			config: ServerOptions{
				Network: "unix",
				Addr:    socketFile,
				Keys:    []gossh.PublicKey{sshPubKey},
			},
			setup: func() error {
				// Create a socket file that should be removed
				f, err := os.Create(socketFile)
				if err != nil {
					return err
				}
				return f.Close()
			},
			cleanup: func() error {
				return os.Remove(socketFile)
			},
		},
		{
			name: "bad key should fail",
			config: ServerOptions{
				Network: "tcp",
				Addr:    ":45987",
				Keys:    []gossh.PublicKey{sshBadPubKey},
			},
			wantErr:     true,
			errContains: "ssh: handshake failed",
		},
		{
			name: "good key still good",
			config: ServerOptions{
				Network: "tcp",
				Addr:    ":45987",
				Keys:    []gossh.PublicKey{sshPubKey},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				err := tt.setup()
				require.NoError(t, err)
			}

			if tt.cleanup != nil {
				defer tt.cleanup()
			}

			agent, err := NewAgent("")
			require.NoError(t, err)

			// Start server in a goroutine since it blocks
			errChan := make(chan error, 1)
			go func() {
				errChan <- agent.StartServer(tt.config)
			}()

			// Add a short delay to allow the server to start
			time.Sleep(100 * time.Millisecond)

			// Try to connect to verify server is running
			var client *gossh.Client

			// Choose the appropriate signer based on the test case
			testSigner := signer
			if tt.name == "bad key should fail" {
				testSigner = badSigner
			}

			sshClientConfig := &gossh.ClientConfig{
				User: "a",
				Auth: []gossh.AuthMethod{
					gossh.PublicKeys(testSigner),
				},
				HostKeyCallback: gossh.InsecureIgnoreHostKey(),
				Timeout:         4 * time.Second,
			}

			switch tt.config.Network {
			case "unix":
				client, err = gossh.Dial("unix", tt.config.Addr, sshClientConfig)
			default:
				if !strings.Contains(tt.config.Addr, ":") {
					tt.config.Addr = ":" + tt.config.Addr
				}
				client, err = gossh.Dial("tcp", tt.config.Addr, sshClientConfig)
			}

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)
			client.Close()
		})
	}
}

func TestStartServerDisableSSH(t *testing.T) {
	t.Setenv("BESZEL_AGENT_DISABLE_SSH", "true")

	agent, err := NewAgent("")
	require.NoError(t, err)

	opts := ServerOptions{
		Network: "tcp",
		Addr:    ":45990",
	}

	err = agent.StartServer(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SSH disabled")
}

func TestStopServerDoesNotBlockWhenEventQueueFull(t *testing.T) {
	agent := createTestAgent(t)
	agent.server = &ssh.Server{}
	agent.connectionManager.eventChan = make(chan ConnectionEvent, 1)
	agent.connectionManager.eventChan <- WebSocketConnect

	done := make(chan error, 1)
	go func() {
		done <- agent.StopServer()
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("StopServer blocked on the connection event queue")
	}

	assert.Nil(t, agent.server)
	assert.Equal(t, WebSocketConnect, <-agent.connectionManager.eventChan)
}

func TestSSHConnectionFallbackLifecycle(t *testing.T) {
	t.Setenv("BESZEL_AGENT_DISABLE_SSH", "false")
	agent := createTestAgent(t)
	cm := agent.connectionManager
	cm.eventChan = make(chan ConnectionEvent, 4)

	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(privateKey)
	require.NoError(t, err)
	cm.serverOptions = ServerOptions{
		Network: "tcp",
		Addr:    "127.0.0.1:0",
		Keys:    []gossh.PublicKey{signer.PublicKey()},
	}

	// A WebSocket that closed after its upgrade returned nil must start SSH.
	cm.handleEvent(WebSocketDisconnect)
	agent.serverMu.Lock()
	require.NotNil(t, agent.serverListener)
	addr := agent.serverListener.Addr().String()
	agent.serverMu.Unlock()
	defer func() { _ = agent.StopServer() }()

	clientConfig := &gossh.ClientConfig{
		User:            "hub",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         4 * time.Second,
	}
	client, err := gossh.Dial("tcp", addr, clientConfig)
	require.NoError(t, err)
	defer client.Close()

	// A connection is counted when it starts its first session.
	startSession := func(c *gossh.Client) *gossh.Session {
		session, err := c.NewSession()
		require.NoError(t, err)
		require.NoError(t, session.Shell())
		return session
	}
	session := startSession(client)

	select {
	case <-cm.sshChanged:
		cm.handleSSHChange()
	case <-time.After(5 * time.Second):
		t.Fatal("SSH connection did not notify the manager")
	}
	require.Equal(t, SSHConnected, cm.getState())

	wsAttempt := make(chan struct{}, 1)
	releaseWS := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWS) }) }
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		wsAttempt <- struct{}{}
		<-releaseWS
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer hub.Close()
	defer release()
	t.Setenv("BESZEL_AGENT_HUB_URL", hub.URL)
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")
	cm.wsClient, err = newWebSocketClient(agent)
	require.NoError(t, err)

	// A normal short-lived session must not be mistaken for a lost connection,
	// and further sessions must not count the same connection again.
	_ = session.Close()
	_ = startSession(client).Close()
	select {
	case <-cm.sshChanged:
		t.Fatal("session close unexpectedly changed SSH connection state")
	case <-time.After(100 * time.Millisecond):
	}
	cm.mu.Lock()
	assert.Equal(t, 1, cm.sshConnections, "sessions should not be counted as connections")
	cm.mu.Unlock()

	secondClient, err := gossh.Dial("tcp", addr, clientConfig)
	require.NoError(t, err)
	defer secondClient.Close()
	defer startSession(secondClient).Close()
	require.Eventually(t, func() bool {
		cm.mu.Lock()
		defer cm.mu.Unlock()
		return cm.sshConnections == 2
	}, 5*time.Second, 10*time.Millisecond, "second SSH connection was not counted")
	require.NoError(t, client.Close())
	select {
	case <-cm.sshChanged:
		t.Fatal("closing one of two SSH connections changed SSH connection state")
	case <-time.After(100 * time.Millisecond):
	}
	require.Equal(t, SSHConnected, cm.getState())
	require.NoError(t, secondClient.Close())
	select {
	case <-cm.sshChanged:
		cm.handleSSHChange()
	case <-time.After(5 * time.Second):
		t.Fatal("SSH TCP close did not notify the manager")
	}
	require.Equal(t, Disconnected, cm.getState())
	require.NotNil(t, cm.wsTicker)
	select {
	case <-wsAttempt:
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not retry WebSocket after SSH disconnected")
	}
	// The hub may redial straight away, so the listener must stay open while
	// the WebSocket attempt is pending and after it fails.
	requireSameListener := func(msg string) {
		agent.serverMu.Lock()
		defer agent.serverMu.Unlock()
		require.NotNil(t, agent.serverListener, msg)
		assert.Equal(t, addr, agent.serverListener.Addr().String(), msg)
	}
	requireSameListener("SSH listener should stay open during the WebSocket attempt")
	thirdClient, err := gossh.Dial("tcp", addr, clientConfig)
	require.NoError(t, err, "SSH should accept a redial during the WebSocket attempt")
	require.NoError(t, thirdClient.Close())
	release()
	require.Eventually(t, func() bool {
		return !cm.isConnectingNow()
	}, 5*time.Second, 10*time.Millisecond, "reconnect attempt did not finish")
	requireSameListener("SSH listener should stay open after the WebSocket attempt fails")
	cm.stopWsTicker()
}

// offeredKeySigner offers an authorized public key without proving possession
// of its private key: Sign blocks until released, then signs with another key.
type offeredKeySigner struct {
	gossh.Signer
	publicKey gossh.PublicKey
	signing   chan struct{}
	release   chan struct{}
}

func (s *offeredKeySigner) PublicKey() gossh.PublicKey { return s.publicKey }

func (s *offeredKeySigner) Sign(rand io.Reader, data []byte) (*gossh.Signature, error) {
	close(s.signing)
	<-s.release
	return s.Signer.Sign(rand, data)
}

// The public key handler runs when a key is offered, before the client signs
// anything, so it must not be what marks an SSH connection as established.
func TestSSHPublicKeyOfferIsNotAConnection(t *testing.T) {
	t.Setenv("BESZEL_AGENT_DISABLE_SSH", "false")
	agent := createTestAgent(t)
	cm := agent.connectionManager
	cm.eventChan = make(chan ConnectionEvent, 4)

	newSigner := func() gossh.Signer {
		_, privateKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		signer, err := gossh.NewSignerFromKey(privateKey)
		require.NoError(t, err)
		return signer
	}
	hubKey := newSigner().PublicKey()
	cm.serverOptions = ServerOptions{
		Network: "tcp",
		Addr:    "127.0.0.1:0",
		Keys:    []gossh.PublicKey{hubKey},
	}

	cm.handleEvent(WebSocketDisconnect)
	agent.serverMu.Lock()
	require.NotNil(t, agent.serverListener)
	addr := agent.serverListener.Addr().String()
	agent.serverMu.Unlock()
	defer func() { _ = agent.StopServer() }()

	signer := &offeredKeySigner{
		Signer:    newSigner(),
		publicKey: hubKey,
		signing:   make(chan struct{}),
		release:   make(chan struct{}),
	}
	dialErr := make(chan error, 1)
	go func() {
		client, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
			User:            "hub",
			Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Timeout:         4 * time.Second,
		})
		if client != nil {
			client.Close()
		}
		dialErr <- err
	}()

	// The server has accepted the offered key and is waiting for a signature.
	select {
	case <-signer.signing:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not accept the offered public key")
	}
	select {
	case <-cm.sshChanged:
		t.Fatal("offering a public key changed SSH connection state")
	case <-time.After(100 * time.Millisecond):
	}
	assert.False(t, cm.hasSSHConnection())
	assert.Equal(t, Disconnected, cm.getState())

	close(signer.release)
	require.Error(t, <-dialErr, "a signature from another key must be rejected")
	assert.False(t, cm.hasSSHConnection())
}

// startSSHFallbackServer starts the fallback SSH server for a disconnected
// agent and returns its address and a client config that can authenticate.
func startSSHFallbackServer(t *testing.T) (*Agent, string, *gossh.ClientConfig) {
	t.Helper()
	t.Setenv("BESZEL_AGENT_DISABLE_SSH", "false")
	agent := createTestAgent(t)
	cm := agent.connectionManager
	cm.eventChan = make(chan ConnectionEvent, 4)

	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(privateKey)
	require.NoError(t, err)
	cm.serverOptions = ServerOptions{
		Network: "tcp",
		Addr:    "127.0.0.1:0",
		Keys:    []gossh.PublicKey{signer.PublicKey()},
	}

	cm.startSSHServer()
	agent.serverMu.Lock()
	require.NotNil(t, agent.serverListener)
	addr := agent.serverListener.Addr().String()
	agent.serverMu.Unlock()
	t.Cleanup(func() { _ = agent.StopServer() })

	return agent, addr, &gossh.ClientConfig{
		User:            "hub",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         4 * time.Second,
	}
}

// connectSSHSession dials the agent and starts a session, which is what marks
// the connection as established.
func connectSSHSession(t *testing.T, addr string, config *gossh.ClientConfig) *gossh.Client {
	t.Helper()
	client, err := gossh.Dial("tcp", addr, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	session, err := client.NewSession()
	require.NoError(t, err)
	require.NoError(t, session.Shell())
	return client
}

// handleNextSSHChange applies the next SSH connection notification, as the
// connection manager's event loop would.
func handleNextSSHChange(t *testing.T, cm *ConnectionManager) {
	t.Helper()
	select {
	case <-cm.sshChanged:
		cm.handleSSHChange()
	case <-time.After(5 * time.Second):
		t.Fatal("SSH connection change did not notify the manager")
	}
}

// An agent without a WebSocket client only has SSH, so losing the hub's SSH
// connection must leave the listener in place for it to reconnect.
func TestSSHDisconnectKeepsListenerWithoutWebSocket(t *testing.T) {
	agent, addr, clientConfig := startSSHFallbackServer(t)
	cm := agent.connectionManager
	require.Nil(t, cm.wsClient)
	defer cm.stopWsTicker()

	client := connectSSHSession(t, addr, clientConfig)
	handleNextSSHChange(t, cm)
	require.Equal(t, SSHConnected, cm.getState())

	require.NoError(t, client.Close())
	handleNextSSHChange(t, cm)
	require.Equal(t, Disconnected, cm.getState())
	require.Eventually(t, func() bool {
		return !cm.isConnectingNow()
	}, 5*time.Second, 10*time.Millisecond, "reconnect attempt did not finish")

	agent.serverMu.Lock()
	require.NotNil(t, agent.serverListener, "SSH listener should stay open")
	assert.Equal(t, addr, agent.serverListener.Addr().String(), "SSH listener should not be restarted")
	agent.serverMu.Unlock()

	connectSSHSession(t, addr, clientConfig)
	handleNextSSHChange(t, cm)
	assert.Equal(t, SSHConnected, cm.getState())
}

// A WebSocket attempt that was already in flight can authenticate after SSH has
// connected. WebSocket is preferred, so it takes over and SSH is shut down.
func TestWebSocketTakesOverFromSSH(t *testing.T) {
	agent, addr, clientConfig := startSSHFallbackServer(t)
	cm := agent.connectionManager

	client := connectSSHSession(t, addr, clientConfig)
	handleNextSSHChange(t, cm)
	require.Equal(t, SSHConnected, cm.getState())

	cm.wsClient = &WebSocketClient{
		agent:       agent,
		hubURL:      &url.URL{Host: "localhost:8080"},
		Conn:        &gws.Conn{},
		hubVerified: true,
	}
	cm.handleEvent(WebSocketConnect)
	require.Equal(t, WebSocketConnected, cm.getState())
	agent.serverMu.Lock()
	assert.Nil(t, agent.serverListener, "SSH listener should close once WebSocket takes over")
	agent.serverMu.Unlock()

	closed := make(chan struct{})
	go func() {
		_ = client.Wait()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("SSH connection was not closed when WebSocket took over")
	}

	// The SSH connection closing must not disturb the WebSocket state.
	handleNextSSHChange(t, cm)
	assert.False(t, cm.hasSSHConnection())
	assert.Equal(t, WebSocketConnected, cm.getState())
}

/////////////////////////////////////////////////////////////////
//////////////////// ParseKeys Tests ////////////////////////////
/////////////////////////////////////////////////////////////////

// Helper function to generate a temporary file with content
func createTempFile(content string) (string, error) {
	tmpFile, err := os.CreateTemp("", "ssh_keys_*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		return "", fmt.Errorf("failed to write to temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

// Test case 1: String with a single SSH key
func TestParseSingleKeyFromString(t *testing.T) {
	input := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKCBM91kukN7hbvFKtbpEeo2JXjCcNxXcdBH7V7ADMBo"
	keys, err := ParseKeys(input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d keys", len(keys))
	}
	if keys[0].Type() != "ssh-ed25519" {
		t.Fatalf("Expected key type 'ssh-ed25519', got '%s'", keys[0].Type())
	}
}

// Test case 2: String with multiple SSH keys
func TestParseMultipleKeysFromString(t *testing.T) {
	input := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKCBM91kukN7hbvFKtbpEeo2JXjCcNxXcdBH7V7ADMBo\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJDMtAOQfxDlCxe+A5lVbUY/DHxK1LAF2Z3AV0FYv36D \n #comment\n ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJDMtAOQfxDlCxe+A5lVbUY/DHxK1LAF2Z3AV0FYv36D"
	keys, err := ParseKeys(input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("Expected 3 keys, got %d keys", len(keys))
	}
	if keys[0].Type() != "ssh-ed25519" || keys[1].Type() != "ssh-ed25519" || keys[2].Type() != "ssh-ed25519" {
		t.Fatalf("Unexpected key types: %s, %s, %s", keys[0].Type(), keys[1].Type(), keys[2].Type())
	}
}

// Test case 3: File with a single SSH key
func TestParseSingleKeyFromFile(t *testing.T) {
	content := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKCBM91kukN7hbvFKtbpEeo2JXjCcNxXcdBH7V7ADMBo"
	filePath, err := createTempFile(content)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(filePath) // Clean up the file after the test

	// Read the file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	// Parse the keys
	keys, err := ParseKeys(string(fileContent))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d keys", len(keys))
	}
	if keys[0].Type() != "ssh-ed25519" {
		t.Fatalf("Expected key type 'ssh-ed25519', got '%s'", keys[0].Type())
	}
}

// Test case 4: File with multiple SSH keys
func TestParseMultipleKeysFromFile(t *testing.T) {
	content := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKCBM91kukN7hbvFKtbpEeo2JXjCcNxXcdBH7V7ADMBo\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJDMtAOQfxDlCxe+A5lVbUY/DHxK1LAF2Z3AV0FYv36D \n #comment\n ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJDMtAOQfxDlCxe+A5lVbUY/DHxK1LAF2Z3AV0FYv36D"
	filePath, err := createTempFile(content)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	// defer os.Remove(filePath) // Clean up the file after the test

	// Read the file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	// Parse the keys
	keys, err := ParseKeys(string(fileContent))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("Expected 3 keys, got %d keys", len(keys))
	}
	if keys[0].Type() != "ssh-ed25519" || keys[1].Type() != "ssh-ed25519" || keys[2].Type() != "ssh-ed25519" {
		t.Fatalf("Unexpected key types: %s, %s, %s", keys[0].Type(), keys[1].Type(), keys[2].Type())
	}
}

// Test case 5: Invalid SSH key input
func TestParseInvalidKey(t *testing.T) {
	input := "invalid-key-data"
	_, err := ParseKeys(input)
	if err == nil {
		t.Fatalf("Expected an error for invalid key, got nil")
	}
	expectedErrMsg := "failed to parse key"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Fatalf("Expected error message to contain '%s', got: %v", expectedErrMsg, err)
	}
}

/////////////////////////////////////////////////////////////////
//////////////////// Hub Version Tests //////////////////////////
/////////////////////////////////////////////////////////////////

func TestExtractHubVersion(t *testing.T) {
	tests := []struct {
		name            string
		clientVersion   string
		expectedVersion string
		expectError     bool
	}{
		{
			name:            "valid beszel client version with underscore",
			clientVersion:   "SSH-2.0-beszel_0.11.1",
			expectedVersion: "0.11.1",
			expectError:     false,
		},
		{
			name:            "valid beszel client version with beta",
			clientVersion:   "SSH-2.0-beszel_1.0.0-beta",
			expectedVersion: "1.0.0-beta",
			expectError:     false,
		},
		{
			name:            "valid beszel client version with rc",
			clientVersion:   "SSH-2.0-beszel_0.12.0-rc1",
			expectedVersion: "0.12.0-rc1",
			expectError:     false,
		},
		{
			name:            "different SSH client",
			clientVersion:   "SSH-2.0-OpenSSH_8.0",
			expectedVersion: "8.0",
			expectError:     true,
		},
		{
			name:          "malformed version string without underscore",
			clientVersion: "SSH-2.0-beszel",
			expectError:   true,
		},
		{
			name:          "empty version string",
			clientVersion: "",
			expectError:   true,
		},
		{
			name:            "version string with underscore but no version",
			clientVersion:   "beszel_",
			expectedVersion: "",
			expectError:     true,
		},
		{
			name:            "version with patch and build metadata",
			clientVersion:   "SSH-2.0-beszel_1.2.3+build.123",
			expectedVersion: "1.2.3+build.123",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractHubVersion(tt.clientVersion)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedVersion, result.String())
		})
	}
}

/////////////////////////////////////////////////////////////////
/////////////// Hub Version Detection Tests ////////////////////
/////////////////////////////////////////////////////////////////

func TestGetHubVersion(t *testing.T) {
	agent, err := NewAgent("")
	require.NoError(t, err)

	// Mock SSH context that implements the ssh.Context interface
	mockCtx := &mockSSHContext{
		sessionID:     "test-session-123",
		clientVersion: "SSH-2.0-beszel_0.12.0",
	}

	// Test first call - should extract version
	version := agent.getHubVersion(mockCtx)
	assert.Equal(t, "0.12.0", version.String())

	// Test that version reflects the current client version (no stale caching)
	mockCtx.clientVersion = "SSH-2.0-beszel_0.11.0"
	version = agent.getHubVersion(mockCtx)
	assert.Equal(t, "0.11.0", version.String())

	// Test with invalid version string (non-beszel client)
	mockCtx.clientVersion = "SSH-2.0-OpenSSH_8.0"
	version = agent.getHubVersion(mockCtx)
	assert.Equal(t, "0.0.0", version.String()) // Should be empty version for non-beszel clients

	// Test with no client version
	mockCtx.clientVersion = ""
	version = agent.getHubVersion(mockCtx)
	assert.True(t, version.EQ(semver.Version{})) // Should be empty version
}

// mockSSHContext implements ssh.Context for testing
type mockSSHContext struct {
	context.Context
	sync.Mutex
	sessionID     string
	clientVersion string
}

func (m *mockSSHContext) SessionID() string {
	return m.sessionID
}

func (m *mockSSHContext) ClientVersion() string {
	return m.clientVersion
}

func (m *mockSSHContext) ServerVersion() string {
	return "SSH-2.0-beszel_test"
}

func (m *mockSSHContext) Value(key interface{}) interface{} {
	if key == ssh.ContextKeyClientVersion {
		return m.clientVersion
	}
	return nil
}

func (m *mockSSHContext) User() string                    { return "test-user" }
func (m *mockSSHContext) RemoteAddr() net.Addr            { return nil }
func (m *mockSSHContext) LocalAddr() net.Addr             { return nil }
func (m *mockSSHContext) Permissions() *ssh.Permissions   { return nil }
func (m *mockSSHContext) SetValue(key, value interface{}) {}

/////////////////////////////////////////////////////////////////
/////////////// CBOR vs JSON Encoding Tests ////////////////////
/////////////////////////////////////////////////////////////////

// TestWriteToSessionEncoding tests that writeToSession actually encodes data in the correct format
func TestWriteToSessionEncoding(t *testing.T) {
	tests := []struct {
		name             string
		hubVersion       string
		expectedUsesCbor bool
	}{
		{
			name:             "old hub version should use JSON",
			hubVersion:       "0.11.1",
			expectedUsesCbor: false,
		},
		{
			name:             "non-beta release should use CBOR",
			hubVersion:       "0.12.0",
			expectedUsesCbor: true,
		},
		{
			name:             "even newer hub version should use CBOR",
			hubVersion:       "0.16.4",
			expectedUsesCbor: true,
		},
		{
			name:             "beta version below release threshold should use JSON",
			hubVersion:       "0.12.0-beta0",
			expectedUsesCbor: false,
		},
		// {
		// 	name:             "matching beta version should use CBOR",
		// 	hubVersion:       "0.12.0-beta2",
		// 	expectedUsesCbor: true,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := NewAgent("")
			require.NoError(t, err)

			// Parse the test version
			version, err := semver.Parse(tt.hubVersion)
			require.NoError(t, err)

			// Create test data to encode
			testData := createTestCombinedData()

			var buf strings.Builder
			err = agent.writeToSession(&buf, testData, version)
			require.NoError(t, err)

			encodedData := buf.String()
			require.NotEmpty(t, encodedData)

			// Verify the encoding format by attempting to decode
			if tt.expectedUsesCbor {
				var decodedCbor system.CombinedData
				err = cbor.Unmarshal([]byte(encodedData), &decodedCbor)
				assert.NoError(t, err, "Should be valid CBOR data")

				var decodedJson system.CombinedData
				err = json.Unmarshal([]byte(encodedData), &decodedJson)
				assert.Error(t, err, "Should not be valid JSON data")

				assert.Equal(t, testData.Details.Hostname, decodedCbor.Details.Hostname)
				assert.Equal(t, testData.Stats.Cpu, decodedCbor.Stats.Cpu)
			} else {
				// Should be JSON - try to decode as JSON
				var decodedJson system.CombinedData
				err = json.Unmarshal([]byte(encodedData), &decodedJson)
				assert.NoError(t, err, "Should be valid JSON data")

				var decodedCbor system.CombinedData
				err = cbor.Unmarshal([]byte(encodedData), &decodedCbor)
				assert.Error(t, err, "Should not be valid CBOR data")

				// Verify the decoded JSON data matches our test data
				assert.Equal(t, testData.Details.Hostname, decodedJson.Details.Hostname)
				assert.Equal(t, testData.Stats.Cpu, decodedJson.Stats.Cpu)

				// Verify it looks like JSON (starts with '{' and contains readable field names)
				assert.True(t, strings.HasPrefix(encodedData, "{"), "JSON should start with '{'")
				assert.Contains(t, encodedData, `"info"`, "JSON should contain readable field names")
				assert.Contains(t, encodedData, `"stats"`, "JSON should contain readable field names")
			}
		})
	}
}

// Helper function to create test data for encoding tests
func createTestCombinedData() *system.CombinedData {
	return &system.CombinedData{
		Stats: system.Stats{
			Cpu:       25.5,
			Mem:       8589934592, // 8GB
			MemUsed:   4294967296, // 4GB
			MemPct:    50.0,
			DiskTotal: 1099511627776, // 1TB
			DiskUsed:  549755813888,  // 512GB
			DiskPct:   50.0,
		},
		Details: &system.Details{
			Hostname: "test-host",
		},
		Info: system.Info{
			Uptime:       3600,
			AgentVersion: "0.12.0",
		},
		Containers: []*container.Stats{
			{
				Name: "test-container",
				Cpu:  10.5,
				Mem:  1073741824, // 1GB
			},
		},
	}
}

// TestGetHubVersionConcurrent guards against a regression of the
// "concurrent map writes" panic previously caused by a shared, unsynchronized
// hubVersions cache (see https://github.com/henrygd/beszel/issues/2128).
// getHubVersion no longer shares mutable state between sessions, so calling
// it concurrently from many goroutines must be safe under `go test -race`.
func TestGetHubVersionConcurrent(t *testing.T) {
	agent, err := NewAgent("")
	require.NoError(t, err)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			ctx := &mockSSHContext{
				sessionID:     fmt.Sprintf("session-%d", i),
				clientVersion: "SSH-2.0-beszel_0.12.0",
			}
			version := agent.getHubVersion(ctx)
			assert.Equal(t, "0.12.0", version.String())
		}(i)
	}
	wg.Wait()
}

/////////////////////////////////////////////////////////////////
/////////////// Session Protocol Tests //////////////////////////
/////////////////////////////////////////////////////////////////

// dialTestAgentClient starts an agent SSH server on a private unix socket and
// connects a real SSH client whose ClientVersion mimics a hub of the given
// version, so session-protocol behavior can be exercised end to end.
func dialTestAgentClient(t *testing.T, clientVersion string) *gossh.Client {
	t.Helper()

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(privKey)
	require.NoError(t, err)
	sshPubKey, err := gossh.NewPublicKey(pubKey)
	require.NoError(t, err)

	socketFile := filepath.Join(t.TempDir(), "beszel-test.sock")
	agent, err := NewAgent("")
	require.NoError(t, err)

	errChan := make(chan error, 1)
	go func() {
		errChan <- agent.StartServer(ServerOptions{
			Network: "unix",
			Addr:    socketFile,
			Keys:    []gossh.PublicKey{sshPubKey},
		})
	}()
	t.Cleanup(func() {
		_ = agent.StopServer()
		select {
		case <-errChan:
		default:
		}
	})

	// Wait for the listener instead of a fixed sleep.
	require.Eventually(t, func() bool {
		conn, err := net.Dial("unix", socketFile)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 5*time.Second, 25*time.Millisecond)

	client, err := gossh.Dial("unix", socketFile, &gossh.ClientConfig{
		User:            "u",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         4 * time.Second,
		ClientVersion:   clientVersion,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// waitSessionOutput runs readFn on a session stream and fails the test if it
// does not finish quickly. A healthy session ends in milliseconds; the guard
// exists so a handler that blocks (the bug this suite guards against) fails
// the test instead of hanging it until the 70s idle timeout.
func waitSessionOutput(t *testing.T, readFn func() ([]byte, error)) []byte {
	t.Helper()
	type result struct {
		output []byte
		err    error
	}
	resCh := make(chan result, 1)
	go func() {
		output, err := readFn()
		resCh <- result{output, err}
	}()
	select {
	case res := <-resCh:
		require.NoError(t, res.err)
		return res.output
	case <-time.After(10 * time.Second):
		t.Fatal("session did not complete within 10s; handler appears to be blocked")
		return nil
	}
}

// TestLegacyHubGetsSinglePayload pins the legacy one-shot contract: a hub
// older than MinVersionAgentResponse never sends a request, so the agent must
// write exactly one stats payload, exit with status 0, and end the session
// promptly. Regression target: the handler used to fall through into the
// request decode after the legacy write, read an immediate EOF from the hub's
// empty stdin, and send a second payload via the decode-failure fallback.
// Covers all three routes into the legacy path: a pre-0.13 CBOR hub, a
// pre-0.12 JSON hub, and a client whose version string cannot be parsed.
func TestLegacyHubGetsSinglePayload(t *testing.T) {
	tests := []struct {
		name          string
		clientVersion string
		cborFormat    bool
	}{
		{name: "legacy cbor hub 0.12.0", clientVersion: "SSH-2.0-beszel_0.12.0", cborFormat: true},
		{name: "legacy json hub 0.11.1", clientVersion: "SSH-2.0-beszel_0.11.1", cborFormat: false},
		{name: "unparsable client version", clientVersion: "SSH-2.0-OpenSSH_8.0", cborFormat: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := dialTestAgentClient(t, tt.clientVersion)

			session, err := client.NewSession()
			require.NoError(t, err)
			defer func() { _ = session.Close() }()

			output := waitSessionOutput(t, func() ([]byte, error) {
				return session.Output("")
			})
			require.NotEmpty(t, output, "legacy hub must receive a stats payload")

			// Exactly one payload: decode the first object, then require the
			// stream to be exhausted. A second write would leave bytes here.
			var data system.CombinedData
			if tt.cborFormat {
				dec := cbor.NewDecoder(bytes.NewReader(output))
				require.NoError(t, dec.Decode(&data), "payload must be valid CBOR")
				var extra any
				require.ErrorIs(t, dec.Decode(&extra), io.EOF,
					"legacy hub must receive exactly one payload, got trailing bytes")
			} else {
				dec := json.NewDecoder(bytes.NewReader(output))
				require.NoError(t, dec.Decode(&data), "payload must be valid JSON")
				var extra map[string]any
				require.ErrorIs(t, dec.Decode(&extra), io.EOF,
					"legacy hub must receive exactly one payload, got trailing bytes")
			}
			// ConnectionType is stamped by the SSH path itself, so it is a
			// deterministic (environment-independent) proof that the payload
			// came from the SSH handler.
			assert.Equal(t, system.ConnectionTypeSSH, data.Info.ConnectionType,
				"payload must be stamped as SSH-sourced")

			// Output() returns an *ssh.ExitError when the handler exits
			// non-zero; reaching this point with no error means exit status 0.
			// (An s.Exit(1) from a failed legacy write would surface here.)
		})
	}
}

// TestModernHubRequestResponse guards the new-protocol path against
// regressions from the legacy-path changes: a hub at or above
// MinVersionAgentResponse sends a CBOR request and must get exactly one
// AgentResponse with populated system data and a clean exit.
func TestModernHubRequestResponse(t *testing.T) {
	client := dialTestAgentClient(t, "SSH-2.0-beszel_0.21.0")

	session, err := client.NewSession()
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	stdout, err := session.StdoutPipe()
	require.NoError(t, err)
	stdin, err := session.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, session.Shell())

	// Same request shape the hub's SSH transport sends.
	require.NoError(t, cbor.NewEncoder(stdin).Encode(common.HubRequest[any]{Action: common.GetData}))
	require.NoError(t, stdin.Close())

	output := waitSessionOutput(t, func() ([]byte, error) {
		return io.ReadAll(stdout)
	})

	var resp common.AgentResponse
	require.NoError(t, cbor.Unmarshal(output, &resp), "response must be a single valid CBOR AgentResponse")
	assert.Empty(t, resp.Error)
	require.NotNil(t, resp.SystemData, "GetData response must carry system data")
	assert.Equal(t, system.ConnectionTypeSSH, resp.SystemData.Info.ConnectionType,
		"GetData response must be stamped as SSH-sourced")

	// Wait() returns an *ssh.ExitError on non-zero exit; nil means status 0.
	require.NoError(t, session.Wait())
}
