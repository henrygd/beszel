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
	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
	"golang.org/x/crypto/ssh"
)

const (
	wsDeadline = 70 * time.Second
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
	hubURLStr, exists := GetEnv("HUB_URL")
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
	token, _ := GetEnv("TOKEN")
	if token != "" {
		return token, nil
	}
	// get token from file
	tokenFile, _ := GetEnv("TOKEN_FILE")
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

	// update the hub url to use websocket scheme and api path
	if client.hubURL.Scheme == "https" {
		client.hubURL.Scheme = "wss"
	} else {
		client.hubURL.Scheme = "ws"
	}
	client.hubURL.Path = path.Join(client.hubURL.Path, "api/beszel/agent-connect")

	client.options = &gws.ClientOption{
		Addr:      client.hubURL.String(),
		TlsConfig: &tls.Config{InsecureSkipVerify: true},
		RequestHeader: http.Header{
			"User-Agent": []string{getUserAgent()},
			"X-Token":    []string{client.token},
			"X-Beszel":   []string{beszel.Version},
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

	client.Conn, _, err = gws.NewClient(client, client.getOptions())
	if err != nil {
		return err
	}

	go client.Conn.ReadLoop()

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
	slog.Warn("Connection closed", "err", strings.TrimPrefix(err.Error(), "gws: "))
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

	if err := cbor.NewDecoder(message.Data).Decode(client.hubRequest); err != nil {
		slog.Error("Error parsing message", "err", err)
		return
	}
	if err := client.handleHubRequest(client.hubRequest); err != nil {
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
func (client *WebSocketClient) handleAuthChallenge(msg *common.HubRequest[cbor.RawMessage]) (err error) {
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
		response.Hostname = client.agent.systemInfo.Hostname
		serverAddr := client.agent.connectionManager.serverOptions.Addr
		_, response.Port, _ = net.SplitHostPort(serverAddr)
	}

	return client.sendMessage(response)
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

// handleHubRequest routes the request to the appropriate handler.
// It ensures the hub is verified before processing most requests.
func (client *WebSocketClient) handleHubRequest(msg *common.HubRequest[cbor.RawMessage]) error {
	if !client.hubVerified && msg.Action != common.CheckFingerprint {
		return errors.New("hub not verified")
	}
	switch msg.Action {
	case common.GetData:
		return client.sendSystemData()
	case common.CheckFingerprint:
		return client.handleAuthChallenge(msg)
	}
	return nil
}

// sendSystemData gathers and sends current system statistics to the hub.
func (client *WebSocketClient) sendSystemData() error {
	sysStats := client.agent.gatherStats(client.token)
	return client.sendMessage(sysStats)
}

// sendMessage encodes the given data to CBOR and sends it as a binary message over the WebSocket connection to the hub.
func (client *WebSocketClient) sendMessage(data any) error {
	bytes, err := cbor.Marshal(data)
	if err != nil {
		return err
	}
	return client.Conn.WriteMessage(gws.OpcodeBinary, bytes)
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
