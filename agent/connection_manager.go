package agent

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/henrygd/beszel/agent/health"
	"github.com/henrygd/beszel/agent/utils"
)

// ConnectionManager manages the connection state and events for the agent.
// It handles both WebSocket and SSH connections, automatically switching between
// them based on availability and managing reconnection attempts.
type ConnectionManager struct {
	agent       *Agent          // Reference to the parent agent
	ctx         context.Context // Lifetime of the running manager
	reconnectWG sync.WaitGroup  // Reconnect attempts must finish before shutdown
	// mu guards state shared by the event loop, connection attempts and SSH callbacks.
	mu             sync.Mutex
	State          ConnectionState      // Current connection state
	eventChan      chan ConnectionEvent // Channel for connection events
	sshChanged     chan struct{}        // Coalesced, nonblocking SSH connection notifications
	wsClient       *WebSocketClient     // WebSocket client for hub communication
	serverOptions  ServerOptions        // Configuration for SSH server
	wsTicker       *time.Ticker         // Ticker for WebSocket connection attempts
	isConnecting   bool                 // Prevents multiple simultaneous reconnection attempts
	sshConnections int                  // Authenticated SSH TCP connections, not sessions
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
		agent:      agent,
		ctx:        context.Background(),
		State:      Disconnected,
		sshChanged: make(chan struct{}, 1),
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

// getState returns the current connection state.
func (c *ConnectionManager) getState() ConnectionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.State
}

func (c *ConnectionManager) hasSSHConnection() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sshConnections > 0
}

func (c *ConnectionManager) notifySSHChange() {
	select {
	case c.sshChanged <- struct{}{}:
	default:
	}
}

// sshConnectionTrackedKey marks an SSH connection context as already counted.
type sshConnectionTrackedKey struct{}

// sshConnectionOpened tracks the authenticated TCP connection. Individual SSH
// sessions are short-lived and must not trigger a return to WebSocket.
//
// It is called from the session handler rather than the public key handler,
// which runs when a key is offered and before the client has proven it holds
// the private key. A connection is counted once however many sessions it opens.
func (c *ConnectionManager) sshConnectionOpened(ctx ssh.Context) {
	ctx.Lock()
	tracked := ctx.Value(sshConnectionTrackedKey{}) != nil
	if !tracked {
		ctx.SetValue(sshConnectionTrackedKey{}, true)
	}
	ctx.Unlock()
	if tracked {
		return
	}

	c.mu.Lock()
	c.sshConnections++
	first := c.sshConnections == 1
	c.mu.Unlock()
	if first {
		c.notifySSHChange()
	}
	go func() {
		<-ctx.Done()
		c.mu.Lock()
		c.sshConnections--
		last := c.sshConnections == 0
		c.mu.Unlock()
		if last {
			c.notifySSHChange()
		}
	}()
}

// setConnecting sets the isConnecting flag and reports its previous value.
func (c *ConnectionManager) setConnecting(v bool) (previous bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous = c.isConnecting
	c.isConnecting = v
	return previous
}

// isConnectingNow reports whether a reconnection attempt is currently in flight.
func (c *ConnectionManager) isConnectingNow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isConnecting
}

// Start begins connection attempts and enters the main event loop.
// It handles connection events, periodic health updates, and graceful shutdown.
func (c *ConnectionManager) Start(serverOptions ServerOptions) error {
	return c.start(context.Background(), serverOptions)
}

func (c *ConnectionManager) start(ctx context.Context, serverOptions ServerOptions) error {
	if c.eventChan != nil {
		return errors.New("already started")
	}

	wsClient, err := newWebSocketClient(c.agent)
	if err != nil {
		var caCertErr *caCertFileError
		if errors.As(err, &caCertErr) {
			return err
		}
		disableSSH, _ := utils.GetEnv("DISABLE_SSH")
		if errors.Is(err, errNoHubURL) && disableSSH != "true" {
			// SSH-only mode: the hub dials the agent, so there is nothing to warn
			// about. With SSH also disabled there is no connection method at all,
			// so that case still warns.
			slog.Debug("WebSocket client not configured", "err", err)
		} else {
			slog.Warn("Error creating WebSocket client", "err", err)
		}
	}
	c.wsClient = wsClient

	c.serverOptions = serverOptions
	c.eventChan = make(chan ConnectionEvent, 1)

	// signal handling for shutdown
	sigCtx, stopSignals := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	c.ctx = sigCtx

	c.startWsTicker()
	defer c.stopWsTicker()
	c.connect()

	// update health status immediately and every 90 seconds
	_ = health.Update()
	healthTicker := time.NewTicker(90 * time.Second)
	defer healthTicker.Stop()

	for {
		select {
		case connectionEvent := <-c.eventChan:
			c.handleEvent(connectionEvent)
		case <-c.sshChanged:
			c.handleSSHChange()
		case <-c.wsTicker.C:
			// skip if connect() is still running its own attempt
			if !c.isConnectingNow() {
				if err := c.startWebSocketConnection(); err != nil {
					c.startSSHServer()
				}
			}
		case <-healthTicker.C:
			_ = health.Update()
		case <-sigCtx.Done():
			slog.Info("Shutting down", "cause", context.Cause(sigCtx))
			c.reconnectWG.Wait()
			return c.stop()
		}
	}
}

func (c *ConnectionManager) handleSSHChange() {
	if c.hasSSHConnection() {
		c.handleEvent(SSHConnect)
	} else {
		c.handleEvent(SSHDisconnect)
	}
}

// stop closes active connections after the manager and reconnect attempts finish.
func (c *ConnectionManager) stop() error {
	_ = c.agent.StopServer()
	c.agent.monitorManager.Stop()
	c.closeWebSocket()
	c.agent.cleanupSensorShadow()
	return health.CleanUp()
}

// handleEvent processes connection events and updates the connection state accordingly.
func (c *ConnectionManager) handleEvent(event ConnectionEvent) {
	switch event {
	case WebSocketConnect:
		if c.wsClient == nil || !c.wsClient.isVerified() {
			return // a superseded connection authenticated after a new attempt began
		}
		// WebSocket is preferred, so it takes over even if an attempt that was
		// already in flight authenticates after SSH has connected.
		c.handleStateChange(WebSocketConnected)
	case SSHConnect:
		if c.getState() == Disconnected && c.hasSSHConnection() {
			c.handleStateChange(SSHConnected)
		}
	case WebSocketDisconnect:
		if c.wsClient != nil && c.wsClient.getConn() != nil {
			return // an older connection closed after its replacement was installed
		}
		if c.getState() == WebSocketConnected {
			c.handleStateChange(Disconnected)
		} else if c.getState() == Disconnected {
			// The WebSocket upgrade can succeed before authentication fails.
			// In that case Connect returned nil, so its error path cannot start SSH.
			c.startSSHServer()
		}
	case SSHDisconnect:
		if c.getState() == SSHConnected && !c.hasSSHConnection() {
			c.handleStateChange(Disconnected)
		}
	}
}

// handleStateChange updates the connection state and performs necessary actions
// based on the new state, including stopping services and initiating reconnections.
func (c *ConnectionManager) handleStateChange(newState ConnectionState) {
	c.mu.Lock()
	if c.State == newState {
		c.mu.Unlock()
		return
	}
	c.State = newState
	c.mu.Unlock()

	switch newState {
	case WebSocketConnected:
		slog.Info("WebSocket connected", "host", c.wsClient.hubURL.Host)
		c.stopWsTicker()
		_ = c.agent.StopServer()
	case SSHConnected:
		// stop new ws connection attempts
		slog.Info("SSH connection established")
		c.stopWsTicker()
	case Disconnected:
		// Listen for SSH whenever disconnected so the hub can fall back to it
		// or redial straight away. WebSocket is still tried first below and
		// stops the server if it connects.
		c.startSSHServer()
		// Always keep the ticker running while disconnected. A pending WebSocket
		// handshake started by connect() can fail asynchronously (e.g. the hub
		// closes the socket, or the deadline set in OnOpen expires) after
		// connect() has already returned with a nil error, in which case the
		// ticker would otherwise never get re-armed and the agent would stop
		// retrying entirely (#2326).
		c.startWsTicker()
		if c.setConnecting(true) {
			// Already handling reconnection, avoid duplicate attempts
			return
		}
		slog.Warn("Disconnected from hub")
		// make sure old ws connection is closed
		c.closeWebSocket()
		// reconnect
		c.reconnectWG.Go(c.connect)
	}
}

// connect handles the connection logic with proper delays and priority.
// It attempts WebSocket connection first, falling back to SSH server if needed.
func (c *ConnectionManager) connect() {
	c.setConnecting(true)
	defer c.setConnecting(false)

	if c.wsClient != nil && time.Since(c.wsClient.lastConnectAttempt) < 5*time.Second {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-c.ctx.Done():
			return
		}
	}

	// Try WebSocket first, if it fails, start SSH server
	err := c.startWebSocketConnection()
	if err != nil {
		if shouldExitOnErr(err) {
			time.Sleep(2 * time.Second) // prevent tight restart loop
			_ = c.stop()
			os.Exit(1)
		}
		if c.getState() == Disconnected {
			c.startSSHServer()
		}
	}
}

// startWebSocketConnection attempts to establish a WebSocket connection to the hub.
func (c *ConnectionManager) startWebSocketConnection() error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if c.getState() != Disconnected {
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
	if c.ctx.Err() != nil {
		return
	}
	c.mu.Lock()
	if c.State != Disconnected {
		c.mu.Unlock()
		return
	}
	if disabled, _ := utils.GetEnv("DISABLE_SSH"); disabled == "true" {
		c.mu.Unlock()
		return
	}
	server, listener, err := c.agent.prepareSSHServer(c.serverOptions)
	c.mu.Unlock()
	if err != nil {
		if !errors.Is(err, errSSHServerRunning) {
			slog.Warn("SSH server failed to start", "err", err)
		}
		return
	}
	go func() {
		if err := c.agent.serveSSHServer(server, listener); err != nil && !errors.Is(err, ssh.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			slog.Warn("SSH server stopped", "err", err)
		}
	}()
}

// sendEvent releases callbacks when the manager has stopped receiving events.
func (c *ConnectionManager) sendEvent(event ConnectionEvent) {
	select {
	case c.eventChan <- event:
	case <-c.ctx.Done():
	}
}

// closeWebSocket closes the WebSocket connection if it exists.
func (c *ConnectionManager) closeWebSocket() {
	if c.wsClient != nil {
		c.wsClient.Close()
	}
}

// shouldExitOnErr checks if the error is a DNS resolution failure and if the
// EXIT_ON_DNS_ERROR env var is set. https://github.com/henrygd/beszel/issues/1924.
func shouldExitOnErr(err error) bool {
	if val, _ := utils.GetEnv("EXIT_ON_DNS_ERROR"); val == "true" {
		if opErr, ok := errors.AsType[*net.OpError](err); ok {
			return strings.Contains(opErr.Err.Error(), "lookup")
		}
	}
	return false
}
