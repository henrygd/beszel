package agent

import (
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/henrygd/beszel/agent/health"
	"github.com/henrygd/beszel/internal/entities/system"
)

// ConnectionManager manages the connection state and events for the agent.
// It handles both WebSocket and SSH connections, automatically switching between
// them based on availability and managing reconnection attempts.
type ConnectionManager struct {
	agent          *Agent               // Reference to the parent agent
	State          ConnectionState      // Current connection state
	eventChan      chan ConnectionEvent // Channel for connection events
	wsClient       *WebSocketClient     // WebSocket client for hub communication
	serverOptions  ServerOptions        // Configuration for SSH server
	wsTicker       *time.Ticker         // Ticker for WebSocket connection attempts
	isConnecting   bool                 // Prevents multiple simultaneous reconnection attempts
	ConnectionType system.ConnectionType
}

// ConnectionState represents the current connection state of the agent.
type ConnectionState uint8

// ConnectionEvent represents connection-related events that can occur.
type ConnectionEvent uint8

// Connection states
const (
	Disconnected       ConnectionState = iota // No active connection
	WebSocketConnected                        // Connected via WebSocket
	SSHConnected                              // Connected via SSH
)

// Connection events
const (
	WebSocketConnect    ConnectionEvent = iota // WebSocket connection established
	WebSocketDisconnect                        // WebSocket connection lost
	SSHConnect                                 // SSH connection established
	SSHDisconnect                              // SSH connection lost
)

const wsTickerInterval = 10 * time.Second

// newConnectionManager creates a new connection manager for the given agent.
func newConnectionManager(agent *Agent) *ConnectionManager {
	cm := &ConnectionManager{
		agent: agent,
		State: Disconnected,
	}
	return cm
}

// startWsTicker starts or resets the WebSocket connection attempt ticker.
func (c *ConnectionManager) startWsTicker() {
	if c.wsTicker == nil {
		c.wsTicker = time.NewTicker(wsTickerInterval)
	} else {
		c.wsTicker.Reset(wsTickerInterval)
	}
}

// stopWsTicker stops the WebSocket connection attempt ticker.
func (c *ConnectionManager) stopWsTicker() {
	if c.wsTicker != nil {
		c.wsTicker.Stop()
	}
}

// Start begins connection attempts and enters the main event loop.
// It handles connection events, periodic health updates, and graceful shutdown.
func (c *ConnectionManager) Start(serverOptions ServerOptions) error {
	if c.eventChan != nil {
		return errors.New("already started")
	}

	wsClient, err := newWebSocketClient(c.agent)
	if err != nil {
		slog.Warn("Error creating WebSocket client", "err", err)
	}
	c.wsClient = wsClient

	c.serverOptions = serverOptions
	c.eventChan = make(chan ConnectionEvent, 1)

	// signal handling for shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	c.startWsTicker()
	c.connect()

	// update health status immediately and every 90 seconds
	_ = health.Update()
	healthTicker := time.Tick(90 * time.Second)

	for {
		select {
		case connectionEvent := <-c.eventChan:
			c.handleEvent(connectionEvent)
		case <-c.wsTicker.C:
			_ = c.startWebSocketConnection()
		case <-healthTicker:
			_ = health.Update()
		case <-sigChan:
			slog.Info("Shutting down")
			_ = c.agent.StopServer()
			c.closeWebSocket()
			return health.CleanUp()
		}
	}
}

// handleEvent processes connection events and updates the connection state accordingly.
func (c *ConnectionManager) handleEvent(event ConnectionEvent) {
	switch event {
	case WebSocketConnect:
		c.handleStateChange(WebSocketConnected)
	case SSHConnect:
		c.handleStateChange(SSHConnected)
	case WebSocketDisconnect:
		if c.State == WebSocketConnected {
			c.handleStateChange(Disconnected)
		}
	case SSHDisconnect:
		if c.State == SSHConnected {
			c.handleStateChange(Disconnected)
		}
	}
}

// handleStateChange updates the connection state and performs necessary actions
// based on the new state, including stopping services and initiating reconnections.
func (c *ConnectionManager) handleStateChange(newState ConnectionState) {
	if c.State == newState {
		return
	}
	c.State = newState
	switch newState {
	case WebSocketConnected:
		slog.Info("WebSocket connected", "host", c.wsClient.hubURL.Host)
		c.ConnectionType = system.ConnectionTypeWebSocket
		c.stopWsTicker()
		_ = c.agent.StopServer()
		c.isConnecting = false
	case SSHConnected:
		// stop new ws connection attempts
		slog.Info("SSH connection established")
		c.ConnectionType = system.ConnectionTypeSSH
		c.stopWsTicker()
		c.isConnecting = false
	case Disconnected:
		c.ConnectionType = system.ConnectionTypeNone
		if c.isConnecting {
			// Already handling reconnection, avoid duplicate attempts
			return
		}
		c.isConnecting = true
		slog.Warn("Disconnected from hub")
		// make sure old ws connection is closed
		c.closeWebSocket()
		// reconnect
		go c.connect()
	}
}

// connect handles the connection logic with proper delays and priority.
// It attempts WebSocket connection first, falling back to SSH server if needed.
func (c *ConnectionManager) connect() {
	c.isConnecting = true
	defer func() {
		c.isConnecting = false
	}()

	if c.wsClient != nil && time.Since(c.wsClient.lastConnectAttempt) < 5*time.Second {
		time.Sleep(5 * time.Second)
	}

	// Try WebSocket first, if it fails, start SSH server
	err := c.startWebSocketConnection()
	if err != nil && c.State == Disconnected {
		c.startSSHServer()
		c.startWsTicker()
	}
}

// startWebSocketConnection attempts to establish a WebSocket connection to the hub.
func (c *ConnectionManager) startWebSocketConnection() error {
	if c.State != Disconnected {
		return errors.New("already connected")
	}
	if c.wsClient == nil {
		return errors.New("WebSocket client not initialized")
	}
	if time.Since(c.wsClient.lastConnectAttempt) < 5*time.Second {
		return errors.New("already connecting")
	}

	err := c.wsClient.Connect()
	if err != nil {
		slog.Warn("WebSocket connection failed", "err", err)
		c.closeWebSocket()
	}
	return err
}

// startSSHServer starts the SSH server if the agent is currently disconnected.
func (c *ConnectionManager) startSSHServer() {
	if c.State == Disconnected {
		go c.agent.StartServer(c.serverOptions)
	}
}

// closeWebSocket closes the WebSocket connection if it exists.
func (c *ConnectionManager) closeWebSocket() {
	if c.wsClient != nil {
		c.wsClient.Close()
	}
}
