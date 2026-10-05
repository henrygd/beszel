package agent

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

const (
	// Keep the connection alive long enough for a slow collection cycle to
	// finish before the hub considers the agent disconnected.
	wsDeadline = 120 * time.Second
)

// errNoHubURL is returned when HUB_URL is unset. This is not a failure
// condition: an agent configured with only a public key runs in SSH-only mode,
// where the hub dials the agent and no outbound WebSocket client is expected.
var errNoHubURL = errors.New("HUB_URL environment variable not set")

type caCertFileError struct {
	err error
}

func (e *caCertFileError) Error() string {
	return e.err.Error()
}

func (e *caCertFileError) Unwrap() error {
	return e.err
}

// WebSocketClient manages the WebSocket connection between the agent and hub.
// It handles authentication, message routing, and connection lifecycle management.
type WebSocketClient struct {
	gws.BuiltinEventHandler
	options            *gws.ClientOption                   // WebSocket client configuration options
	agent              *Agent                              // Reference to the parent agent
	connMu             sync.RWMutex                        // Guards Conn and hubVerified across callbacks
	Conn               *gws.Conn                           // Active WebSocket connection
	hubURL             *url.URL                            // Parsed hub URL for connection
	token              string                              // Authentication token for hub registration
	fingerprint        string                              // System fingerprint for identification
	hubRequest         *common.HubRequest[cbor.RawMessage] // Reusable request structure for message parsing
	lastConnectAttempt time.Time                           // Timestamp of last connection attempt
	hubVerified        bool                                // Whether the hub has been cryptographically verified
	tlsConfig          *tls.Config                         // Optional TLS configuration with custom CA certificates
}

// newWebSocketClient creates a new WebSocket client for the given agent.
// It reads configuration from environment variables and validates the hub URL.
func newWebSocketClient(agent *Agent) (client *WebSocketClient, err error) {
	hubURLStr, exists := utils.GetEnv("HUB_URL")
	if !exists {
		return nil, errNoHubURL
	}

	client = &WebSocketClient{}

	client.hubURL, err = url.Parse(hubURLStr)
	if err != nil || client.hubURL.Host == "" {
		return nil, fmt.Errorf("invalid HUB_URL %q: must include scheme and host (e.g. http://hub.example.com:8090)", hubURLStr)
	}
	// get registration token
	client.token, err = getToken()
	if err != nil {
		return nil, err
	}
	client.tlsConfig, err = getTLSConfig()
	if err != nil {
		return nil, err
	}

	client.agent = agent
	client.hubRequest = &common.HubRequest[cbor.RawMessage]{}
	client.fingerprint = agent.getFingerprint()

	return client, nil
}

// getToken returns the token for the WebSocket client.
// It first checks the TOKEN environment variable, then the TOKEN_FILE environment variable.
// If neither is set, it returns an error.
func getToken() (string, error) {
	// get token from env var
	token, _ := utils.GetEnv("TOKEN")
	if token != "" {
		return token, nil
	}
	// get token from file
	tokenFile, _ := utils.GetEnv("TOKEN_FILE")
	if tokenFile == "" {
		return "", errors.New("must set TOKEN or TOKEN_FILE")
	}
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}
	return parseTokenFile(string(tokenBytes), tokenFile)
}

// parseTokenFile reads a single token from TOKEN_FILE.
// Blank lines and comments are ignored. Multiple tokens are rejected because
// the agent supports only one outbound hub connection.
func parseTokenFile(contents, path string) (string, error) {
	var token string
	for line := range strings.Lines(contents) {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		if token != "" {
			return "", fmt.Errorf("%s must contain a single token", path)
		}
		token = line
	}
	// An empty file keeps returning an empty token, as before: the caller decides
	// what to do about it.
	return token, nil
}

// getTLSConfig returns a TLS configuration containing the system certificate
// pool plus any certificates configured through CA_CERT_FILE. A nil config lets
// gws use Go's default TLS configuration and system roots.
func getTLSConfig() (*tls.Config, error) {
	caCertFile, _ := utils.GetEnv("CA_CERT_FILE")
	if caCertFile == "" {
		return nil, nil
	}

	caCertPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, &caCertFileError{fmt.Errorf("read CA_CERT_FILE %q: %w", caCertFile, err)}
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, &caCertFileError{fmt.Errorf("load system CA certificate pool: %w", err)}
	}
	if !rootCAs.AppendCertsFromPEM(caCertPEM) {
		return nil, &caCertFileError{fmt.Errorf("CA_CERT_FILE %q does not contain any valid PEM certificates", caCertFile)}
	}

	return &tls.Config{RootCAs: rootCAs}, nil
}

// getOptions returns the WebSocket client options, creating them if necessary.
// It configures the connection URL, TLS settings, and authentication headers.
func (client *WebSocketClient) getOptions() *gws.ClientOption {
	if client.options != nil {
		return client.options
	}

	// update the hub url to use websocket scheme and api path
	if client.hubURL.Scheme == "https" {
		client.hubURL.Scheme = "wss"
	} else {
		client.hubURL.Scheme = "ws"
	}
	client.hubURL.Path = path.Join(client.hubURL.Path, "api/beszel/agent-connect")

	// make sure BESZEL_AGENT_ALL_PROXY works (GWS only checks ALL_PROXY)
	if val := os.Getenv("BESZEL_AGENT_ALL_PROXY"); val != "" {
		os.Setenv("ALL_PROXY", val)
	}

	client.options = &gws.ClientOption{
		Addr:      client.hubURL.String(),
		TlsConfig: client.tlsConfig,
		RequestHeader: http.Header{
			"User-Agent": []string{getUserAgent()},
			"X-Token":    []string{client.token},
			"X-Beszel":   []string{beszel.Version},
		},
		NewDialer: func() (gws.Dialer, error) {
			return proxy.FromEnvironment(), nil
		},
	}
	return client.options
}

// Connect establishes a WebSocket connection to the hub.
// It closes any existing connection before attempting to reconnect.
func (client *WebSocketClient) Connect() (err error) {
	client.lastConnectAttempt = time.Now()

	// make sure previous connection is closed
	client.Close()

	conn, _, err := gws.NewClient(client, client.getOptions())
	if err != nil {
		return err
	}
	client.connMu.Lock()
	client.Conn = conn
	client.hubVerified = false
	client.connMu.Unlock()

	go conn.ReadLoop()

	return nil
}

// OnOpen handles WebSocket connection establishment.
// It sets a deadline for the connection to prevent hanging.
func (client *WebSocketClient) OnOpen(conn *gws.Conn) {
	conn.SetDeadline(time.Now().Add(wsDeadline))
}

// OnClose handles WebSocket connection closure.
// It logs the closure reason and notifies the connection manager.
func (client *WebSocketClient) OnClose(conn *gws.Conn, err error) {
	client.connMu.Lock()
	if client.Conn != conn {
		client.connMu.Unlock()
		return
	}
	client.Conn = nil
	client.hubVerified = false
	client.connMu.Unlock()
	if err != nil {
		slog.Warn("Connection closed", "err", strings.TrimPrefix(err.Error(), "gws: "))
	}
	client.agent.connectionManager.eventChan <- WebSocketDisconnect
}

// OnMessage handles incoming WebSocket messages from the hub.
// It decodes CBOR messages and routes them to appropriate handlers.
func (client *WebSocketClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
	if client.getConn() != conn {
		return
	}
	conn.SetDeadline(time.Now().Add(wsDeadline))

	if message.Opcode != gws.OpcodeBinary {
		return
	}

	var HubRequest common.HubRequest[cbor.RawMessage]

	err := cbor.Unmarshal(message.Data.Bytes(), &HubRequest)
	if err != nil {
		slog.Error("Error parsing message", "err", err)
		return
	}

	if err := client.handleHubRequest(&HubRequest, HubRequest.Id, conn); err != nil {
		slog.Error("Error handling message", "err", err)
	}
}

// OnPing handles WebSocket ping frames.
// It responds with a pong and updates the connection deadline.
func (client *WebSocketClient) OnPing(conn *gws.Conn, message []byte) {
	conn.SetDeadline(time.Now().Add(wsDeadline))
	conn.WritePong(message)
}

// handleAuthChallenge verifies the authenticity of the hub and returns the system's fingerprint.
func (client *WebSocketClient) handleAuthChallenge(msg *common.HubRequest[cbor.RawMessage], requestID *uint32, conn *gws.Conn) (err error) {
	var authRequest common.FingerprintRequest
	if err := cbor.Unmarshal(msg.Data, &authRequest); err != nil {
		return err
	}

	if err := client.verifySignature(authRequest.Signature); err != nil {
		return err
	}

	client.connMu.Lock()
	if conn != nil && client.Conn != conn {
		client.connMu.Unlock()
		return gws.ErrConnClosed
	}
	client.hubVerified = true
	client.connMu.Unlock()
	client.agent.connectionManager.eventChan <- WebSocketConnect

	response := &common.FingerprintResponse{
		Fingerprint: client.fingerprint,
	}

	if authRequest.NeedSysInfo {
		response.Name, _ = utils.GetEnv("SYSTEM_NAME")
		response.Hostname = client.agent.systemDetails.Hostname
		serverAddr := client.agent.connectionManager.serverOptions.Addr
		_, response.Port, _ = net.SplitHostPort(serverAddr)
	}

	if conn != nil {
		return client.sendResponseOnConn(conn, response, requestID)
	}
	return client.sendResponse(response, requestID)
}

// verifySignature verifies the signature of the token using the public keys.
func (client *WebSocketClient) verifySignature(signature []byte) (err error) {
	for _, pubKey := range client.agent.keys {
		sig := ssh.Signature{
			Format: pubKey.Type(),
			Blob:   signature,
		}
		if err = pubKey.Verify([]byte(client.token), &sig); err == nil {
			return nil
		}
	}
	return errors.New("invalid signature - check KEY value")
}

// Close closes the WebSocket connection gracefully.
// This method is safe to call multiple times.
func (client *WebSocketClient) Close() {
	if conn := client.getConn(); conn != nil {
		_ = conn.WriteClose(1000, nil)
	}
}

func (client *WebSocketClient) getConn() *gws.Conn {
	client.connMu.RLock()
	defer client.connMu.RUnlock()
	return client.Conn
}

func (client *WebSocketClient) isVerified() bool {
	client.connMu.RLock()
	defer client.connMu.RUnlock()
	return client.Conn != nil && client.hubVerified
}

// handleHubRequest routes the request to the appropriate handler using the handler registry.
func (client *WebSocketClient) handleHubRequest(msg *common.HubRequest[cbor.RawMessage], requestID *uint32, conn *gws.Conn) error {
	client.connMu.RLock()
	verified := client.hubVerified
	client.connMu.RUnlock()
	sendResponse := client.sendResponse
	if conn != nil {
		sendResponse = func(data any, requestID *uint32) error {
			return client.sendResponseOnConn(conn, data, requestID)
		}
	}
	ctx := &HandlerContext{
		Client:         client,
		Conn:           conn,
		Agent:          client.agent,
		Request:        msg,
		RequestID:      requestID,
		HubVerified:    verified,
		ConnectionType: system.ConnectionTypeWebSocket,
		SendResponse:   sendResponse,
	}
	return client.agent.handlerRegistry.Handle(ctx)
}

// sendMessage encodes the given data to CBOR and sends it as a binary message over the WebSocket connection to the hub.
func (client *WebSocketClient) sendMessage(data any) error {
	return client.sendMessageOnConn(client.getConn(), data)
}

func (client *WebSocketClient) sendMessageOnConn(conn *gws.Conn, data any) error {
	bytes, err := cbor.Marshal(data)
	if err != nil {
		return err
	}
	if conn == nil {
		return gws.ErrConnClosed
	}
	err = conn.WriteMessage(gws.OpcodeBinary, bytes)
	if err != nil {
		// If writing fails (e.g., broken pipe due to network issues),
		// close the connection to trigger reconnection logic (#1263)
		_ = conn.WriteClose(1000, nil)
	}
	return err
}

// sendResponse sends a response with optional request ID.
// For ID-based requests, we must populate legacy typed fields for backward
// compatibility with older hubs (<= 0.17) that don't read the generic Data field.
func (client *WebSocketClient) sendResponse(data any, requestID *uint32) error {
	return client.sendResponseOnConn(client.getConn(), data, requestID)
}

func (client *WebSocketClient) sendResponseOnConn(conn *gws.Conn, data any, requestID *uint32) error {
	if requestID != nil {
		response := newAgentResponse(data, requestID)
		return client.sendMessageOnConn(conn, response)
	}
	// Legacy format - send data directly
	return client.sendMessageOnConn(conn, data)
}

// getUserAgent returns one of two User-Agent strings based on current time.
// This is used to avoid being blocked by Cloudflare or other anti-bot measures.
func getUserAgent() string {
	const (
		uaBase    = "Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
		uaWindows = "Windows NT 11.0; Win64; x64"
		uaMac     = "Macintosh; Intel Mac OS X 14_0_0"
	)
	if time.Now().UnixNano()%2 == 0 {
		return fmt.Sprintf(uaBase, uaWindows)
	}
	return fmt.Sprintf(uaBase, uaMac)
}
