package agent

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

const (
	wsDeadline        = 70 * time.Second
	maxWSRedirects    = 5
	agentConnectRoute = "api/beszel/agent-connect"
)

// WebSocketClient manages the WebSocket connection between the agent and hub.
// It handles authentication, message routing, and connection lifecycle management.
type WebSocketClient struct {
	gws.BuiltinEventHandler
	options            *gws.ClientOption                   // WebSocket client configuration options
	agent              *Agent                              // Reference to the parent agent
	Conn               *gws.Conn                           // Active WebSocket connection
	hubURL             *url.URL                            // Parsed hub URL for connection
	token              string                              // Authentication token for hub registration
	fingerprint        string                              // System fingerprint for identification
	hubRequest         *common.HubRequest[cbor.RawMessage] // Reusable request structure for message parsing
	lastConnectAttempt time.Time                           // Timestamp of last connection attempt
	hubVerified        bool                                // Whether the hub has been cryptographically verified
}

// newWebSocketClient creates a new WebSocket client for the given agent.
// It reads configuration from environment variables and validates the hub URL.
func newWebSocketClient(agent *Agent) (client *WebSocketClient, err error) {
	hubURLStr, exists := utils.GetEnv("HUB_URL")
	if !exists {
		return nil, errors.New("HUB_URL environment variable not set")
	}

	client = &WebSocketClient{}

	client.hubURL, err = url.Parse(hubURLStr)
	if err != nil {
		return nil, errors.New("invalid hub URL")
	}
	// get registration token
	client.token, err = getToken()
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
	return strings.TrimSpace(string(tokenBytes)), nil
}

// getOptions returns the WebSocket client options, creating them if necessary.
// It configures the connection URL, TLS settings, and authentication headers.
func (client *WebSocketClient) getOptions() *gws.ClientOption {
	if client.options != nil {
		return client.options
	}

	hubURL := *client.hubURL
	hubURL.Path = path.Join(hubURL.Path, agentConnectRoute)
	updateURLToWebSocketScheme(&hubURL)

	// make sure BESZEL_AGENT_ALL_PROXY works (GWS only checks ALL_PROXY)
	if val := os.Getenv("BESZEL_AGENT_ALL_PROXY"); val != "" {
		os.Setenv("ALL_PROXY", val)
	}

	client.options = &gws.ClientOption{
		Addr:      hubURL.String(),
		TlsConfig: &tls.Config{InsecureSkipVerify: true},
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

	options := client.getOptions()
	for redirectCount := 0; ; redirectCount++ {
		var resp *http.Response
		client.Conn, resp, err = gws.NewClient(client, options)
		if err == nil {
			break
		}

		nextAddr, redirected, redirectErr := resolveWebSocketRedirect(options.Addr, resp)
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if redirectErr != nil {
			return fmt.Errorf("%w: %v", err, redirectErr)
		}
		if !redirected {
			return err
		}
		if redirectCount >= maxWSRedirects {
			return fmt.Errorf("%w: stopped after %d websocket redirects", err, maxWSRedirects)
		}

		slog.Info("Following WebSocket redirect", "from", options.Addr, "to", nextAddr)
		options.Addr = nextAddr
	}

	go client.Conn.ReadLoop()

	return nil
}

func resolveWebSocketRedirect(currentAddr string, resp *http.Response) (string, bool, error) {
	if resp == nil || !isHTTPRedirect(resp.StatusCode) {
		return "", false, nil
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", false, fmt.Errorf("websocket redirect status %d missing Location header", resp.StatusCode)
	}

	baseURL, err := url.Parse(currentAddr)
	if err != nil {
		return "", false, fmt.Errorf("invalid current websocket URL: %w", err)
	}

	redirectURL, err := baseURL.Parse(location)
	if err != nil {
		return "", false, fmt.Errorf("invalid websocket redirect URL %q: %w", location, err)
	}
	updateURLToWebSocketScheme(redirectURL)
	return redirectURL.String(), true, nil
}

func isHTTPRedirect(statusCode int) bool {
	switch statusCode {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func updateURLToWebSocketScheme(u *url.URL) {
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
}

// OnOpen handles WebSocket connection establishment.
// It sets a deadline for the connection to prevent hanging.
func (client *WebSocketClient) OnOpen(conn *gws.Conn) {
	conn.SetDeadline(time.Now().Add(wsDeadline))
}

// OnClose handles WebSocket connection closure.
// It logs the closure reason and notifies the connection manager.
func (client *WebSocketClient) OnClose(conn *gws.Conn, err error) {
	if err != nil {
		slog.Warn("Connection closed", "err", strings.TrimPrefix(err.Error(), "gws: "))
	}
	client.agent.connectionManager.eventChan <- WebSocketDisconnect
}

// OnMessage handles incoming WebSocket messages from the hub.
// It decodes CBOR messages and routes them to appropriate handlers.
func (client *WebSocketClient) OnMessage(conn *gws.Conn, message *gws.Message) {
	defer message.Close()
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

	if err := client.handleHubRequest(&HubRequest, HubRequest.Id); err != nil {
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
func (client *WebSocketClient) handleAuthChallenge(msg *common.HubRequest[cbor.RawMessage], requestID *uint32) (err error) {
	var authRequest common.FingerprintRequest
	if err := cbor.Unmarshal(msg.Data, &authRequest); err != nil {
		return err
	}

	if err := client.verifySignature(authRequest.Signature); err != nil {
		return err
	}

	client.hubVerified = true
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
	if client.Conn != nil {
		_ = client.Conn.WriteClose(1000, nil)
	}
}

// handleHubRequest routes the request to the appropriate handler using the handler registry.
func (client *WebSocketClient) handleHubRequest(msg *common.HubRequest[cbor.RawMessage], requestID *uint32) error {
	ctx := &HandlerContext{
		Client:       client,
		Agent:        client.agent,
		Request:      msg,
		RequestID:    requestID,
		HubVerified:  client.hubVerified,
		SendResponse: client.sendResponse,
	}
	return client.agent.handlerRegistry.Handle(ctx)
}

// sendMessage encodes the given data to CBOR and sends it as a binary message over the WebSocket connection to the hub.
func (client *WebSocketClient) sendMessage(data any) error {
	bytes, err := cbor.Marshal(data)
	if err != nil {
		return err
	}
	err = client.Conn.WriteMessage(gws.OpcodeBinary, bytes)
	if err != nil {
		// If writing fails (e.g., broken pipe due to network issues),
		// close the connection to trigger reconnection logic (#1263)
		client.Close()
	}
	return err
}

// sendResponse sends a response with optional request ID.
// For ID-based requests, we must populate legacy typed fields for backward
// compatibility with older hubs (<= 0.17) that don't read the generic Data field.
func (client *WebSocketClient) sendResponse(data any, requestID *uint32) error {
	if requestID != nil {
		response := newAgentResponse(data, requestID)
		return client.sendMessage(response)
	}
	// Legacy format - send data directly
	return client.sendMessage(data)
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
