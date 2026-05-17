//go:build testing

package agent

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func createTestAgent(t *testing.T) *Agent {
	dataDir := t.TempDir()
	agent, err := NewAgent(dataDir)
	require.NoError(t, err)
	return agent
}

func createTestServerOptions(t *testing.T) ServerOptions {
	// Generate test key pair
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	sshPubKey, err := ssh.NewPublicKey(privKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	// Find available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	return ServerOptions{
		Network: "tcp",
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Keys:    []ssh.PublicKey{sshPubKey},
	}
}

// TestConnectionManager_NewConnectionManager tests connection manager creation
func TestConnectionManager_NewConnectionManager(t *testing.T) {
	agent := createTestAgent(t)
	cm := newConnectionManager(agent)

	assert.NotNil(t, cm, "Connection manager should not be nil")
	assert.Equal(t, agent, cm.agent, "Agent reference should be set")
	assert.Equal(t, Disconnected, cm.State, "Initial state should be Disconnected")
	assert.Nil(t, cm.eventChan, "Event channel should be nil initially")
	assert.Nil(t, cm.wsClient, "WebSocket client should be nil initially")
	assert.Nil(t, cm.wsTicker, "WebSocket ticker should be nil initially")
	assert.False(t, cm.isConnecting, "isConnecting should be false initially")
}

// TestConnectionManager_StateTransitions tests basic state transitions
func TestConnectionManager_StateTransitions(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager
	initialState := cm.State
	cm.wsClient = &WebSocketClient{
		hubURL: &url.URL{
			Host: "localhost:8080",
		},
	}
	assert.NotNil(t, cm, "Connection manager should not be nil")
	assert.Equal(t, Disconnected, initialState, "Initial state should be Disconnected")

	// Test state transitions
	cm.handleStateChange(WebSocketConnected)
	assert.Equal(t, WebSocketConnected, cm.State, "State should change to WebSocketConnected")

	cm.handleStateChange(SSHConnected)
	assert.Equal(t, SSHConnected, cm.State, "State should change to SSHConnected")

	cm.handleStateChange(Disconnected)
	assert.Equal(t, Disconnected, cm.State, "State should change to Disconnected")

	// Test that same state doesn't trigger changes
	cm.State = WebSocketConnected
	cm.handleStateChange(WebSocketConnected)
	assert.Equal(t, WebSocketConnected, cm.State, "Same state should not trigger change")
}

// TestConnectionManager_EventHandling tests event handling logic
func TestConnectionManager_EventHandling(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager
	cm.wsClient = &WebSocketClient{
		hubURL: &url.URL{
			Host: "localhost:8080",
		},
	}

	testCases := []struct {
		name          string
		initialState  ConnectionState
		event         ConnectionEvent
		expectedState ConnectionState
	}{
		{
			name:          "WebSocket connect from disconnected",
			initialState:  Disconnected,
			event:         WebSocketConnect,
			expectedState: WebSocketConnected,
		},
		{
			name:          "SSH connect from disconnected",
			initialState:  Disconnected,
			event:         SSHConnect,
			expectedState: SSHConnected,
		},
		{
			name:          "WebSocket disconnect from connected",
			initialState:  WebSocketConnected,
			event:         WebSocketDisconnect,
			expectedState: Disconnected,
		},
		{
			name:          "SSH disconnect from connected",
			initialState:  SSHConnected,
			event:         SSHDisconnect,
			expectedState: Disconnected,
		},
		{
			name:          "WebSocket disconnect from SSH connected (no change)",
			initialState:  SSHConnected,
			event:         WebSocketDisconnect,
			expectedState: SSHConnected,
		},
		{
			name:          "SSH disconnect from WebSocket connected (no change)",
			initialState:  WebSocketConnected,
			event:         SSHDisconnect,
			expectedState: WebSocketConnected,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cm.State = tc.initialState
			cm.handleEvent(tc.event)
			assert.Equal(t, tc.expectedState, cm.State, "State should match expected after event")
		})
	}
}

// TestConnectionManager_TickerManagement tests WebSocket ticker management
func TestConnectionManager_TickerManagement(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager

	// Test starting ticker
	cm.startWsTicker()
	assert.NotNil(t, cm.wsTicker, "Ticker should be created")

	// Test stopping ticker (should not panic)
	assert.NotPanics(t, func() {
		cm.stopWsTicker()
	}, "Stopping ticker should not panic")

	// Test stopping nil ticker (should not panic)
	cm.wsTicker = nil
	assert.NotPanics(t, func() {
		cm.stopWsTicker()
	}, "Stopping nil ticker should not panic")

	// Test restarting ticker
	cm.startWsTicker()
	assert.NotNil(t, cm.wsTicker, "Ticker should be recreated")

	// Test resetting existing ticker
	firstTicker := cm.wsTicker
	cm.startWsTicker()
	assert.Equal(t, firstTicker, cm.wsTicker, "Same ticker instance should be reused")

	cm.stopWsTicker()
}

// TestConnectionManager_WebSocketConnectionFlow tests WebSocket connection logic
func TestConnectionManager_WebSocketConnectionFlow(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager

	// Test WebSocket connection without proper environment
	err := cm.startWebSocketConnection()
	assert.Error(t, err, "WebSocket connection should fail without proper environment")
	assert.Equal(t, Disconnected, cm.State, "State should remain Disconnected after failed connection")

	// Test with invalid URL
	t.Setenv("BESZEL_AGENT_HUB_URL", "1,33%")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

	_, err2 := newWebSocketClient(agent)
	assert.Error(t, err2, "WebSocket client creation should fail with invalid URL")

	// Test with missing token
	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "")

	_, err3 := newWebSocketClient(agent)
	assert.Error(t, err3, "WebSocket client creation should fail without token")
}

// TestConnectionManager_ReconnectionLogic tests reconnection prevention logic
func TestConnectionManager_ReconnectionLogic(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager
	cm.eventChan = make(chan ConnectionEvent, 1)

	// Test that isConnecting flag prevents duplicate reconnection attempts
	// Start from connected state, then simulate disconnect
	cm.State = WebSocketConnected
	cm.isConnecting = false

	// First disconnect should trigger reconnection logic
	cm.handleStateChange(Disconnected)
	assert.Equal(t, Disconnected, cm.State, "Should change to disconnected")
	assert.True(t, cm.isConnecting, "Should set isConnecting flag")
}

// TestConnectionManager_ConnectWithRateLimit tests connection rate limiting
func TestConnectionManager_ConnectWithRateLimit(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager

	// Set up environment for WebSocket client creation
	t.Setenv("BESZEL_AGENT_HUB_URL", "ws://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

	// Create WebSocket client
	wsClient, err := newWebSocketClient(agent)
	require.NoError(t, err)
	cm.wsClient = wsClient

	// Set recent connection attempt
	cm.wsClient.lastConnectAttempt = time.Now()

	// Test that connection is rate limited
	err = cm.startWebSocketConnection()
	assert.Error(t, err, "Should error due to rate limiting")
	assert.Contains(t, err.Error(), "already connecting", "Error should indicate rate limiting")

	// Test connection after rate limit expires
	cm.wsClient.lastConnectAttempt = time.Now().Add(-10 * time.Second)
	err = cm.startWebSocketConnection()
	// This will fail due to no actual server, but should not be rate limited
	assert.Error(t, err, "Connection should fail but not due to rate limiting")
	assert.NotContains(t, err.Error(), "already connecting", "Error should not indicate rate limiting")
}

// TestConnectionManager_StartWithInvalidConfig tests starting with invalid configuration
func TestConnectionManager_StartWithInvalidConfig(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager
	serverOptions := createTestServerOptions(t)

	// Test starting when already started
	cm.eventChan = make(chan ConnectionEvent, 5)
	err := cm.Start(serverOptions)
	assert.Error(t, err, "Should error when starting already started connection manager")
}

// TestConnectionManager_CloseWebSocket tests WebSocket closing
func TestConnectionManager_CloseWebSocket(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager

	// Test closing when no WebSocket client exists
	assert.NotPanics(t, func() {
		cm.closeWebSocket()
	}, "Should not panic when closing nil WebSocket client")

	// Set up environment and create WebSocket client
	t.Setenv("BESZEL_AGENT_HUB_URL", "ws://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

	wsClient, err := newWebSocketClient(agent)
	require.NoError(t, err)
	cm.wsClient = wsClient

	// Test closing when WebSocket client exists
	assert.NotPanics(t, func() {
		cm.closeWebSocket()
	}, "Should not panic when closing WebSocket client")
}

// TestConnectionManager_ConnectFlow tests the connect method
func TestConnectionManager_ConnectFlow(t *testing.T) {
	agent := createTestAgent(t)
	cm := agent.connectionManager

	// Test connect without WebSocket client
	assert.NotPanics(t, func() {
		cm.connect()
	}, "Connect should not panic without WebSocket client")
}

func TestShouldExitOnErr(t *testing.T) {
	createDialErr := func(msg string) error {
		return &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: errors.New(msg),
		}
	}

	tests := []struct {
		name     string
		err      error
		envValue string
		expected bool
	}{
		{
			name:     "no env var",
			err:      createDialErr("lookup lkahsdfasdf: no such host"),
			envValue: "",
			expected: false,
		},
		{
			name:     "env var false",
			err:      createDialErr("lookup lkahsdfasdf: no such host"),
			envValue: "false",
			expected: false,
		},
		{
			name:     "env var true, matching error",
			err:      createDialErr("lookup lkahsdfasdf: no such host"),
			envValue: "true",
			expected: true,
		},
		{
			name:     "env var true, matching error with extra context",
			err:      createDialErr("lookup beszel.server.lan on [::1]:53: read udp [::1]:44557->[::1]:53: read: connection refused"),
			envValue: "true",
			expected: true,
		},
		{
			name:     "env var true, non-matching error",
			err:      errors.New("connection refused"),
			envValue: "true",
			expected: false,
		},
		{
			name:     "env var true, dial but not lookup",
			err:      createDialErr("connection timeout"),
			envValue: "true",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EXIT_ON_DNS_ERROR", tt.envValue)
			result := shouldExitOnErr(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
