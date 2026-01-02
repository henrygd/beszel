package ws

import (
	"errors"
	"net/http"
	"time"
	"weak"

	"github.com/blang/semver"
	"github.com/henrygd/beszel"

	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
)

const (
	deadline = 70 * time.Second
)

// Handler implements the WebSocket event handler for agent connections.
type Handler struct {
	gws.BuiltinEventHandler
}

// WsConn represents a WebSocket connection to an agent.
type WsConn struct {
	conn           *gws.Conn
	requestManager *RequestManager
	DownChan       chan struct{}
	agentVersion   semver.Version
}

// FingerprintRecord is fingerprints collection record data in the hub
type FingerprintRecord struct {
	Id          string `db:"id"`
	SystemId    string `db:"system"`
	Fingerprint string `db:"fingerprint"`
	Token       string `db:"token"`
}

var upgrader *gws.Upgrader

// GetUpgrader returns a singleton WebSocket upgrader instance.
func GetUpgrader() *gws.Upgrader {
	if upgrader != nil {
		return upgrader
	}
	handler := &Handler{}
	upgrader = gws.NewUpgrader(handler, &gws.ServerOption{
		ResponseHeader: http.Header{
			"X-Accel-Buffering": []string{"no"},
		},
	})
	return upgrader
}

// NewWsConnection creates a new WebSocket connection wrapper with agent version.
func NewWsConnection(conn *gws.Conn, agentVersion semver.Version) *WsConn {
	return &WsConn{
		conn:           conn,
		requestManager: NewRequestManager(conn),
		DownChan:       make(chan struct{}, 1),
		agentVersion:   agentVersion,
	}
}

// OnOpen sets a deadline for the WebSocket connection and extracts agent version.
func (h *Handler) OnOpen(conn *gws.Conn) {
	conn.SetDeadline(time.Now().Add(deadline))
}

// OnMessage routes incoming WebSocket messages to the request manager.
func (h *Handler) OnMessage(conn *gws.Conn, message *gws.Message) {
	conn.SetDeadline(time.Now().Add(deadline))
	if message.Opcode != gws.OpcodeBinary || message.Data.Len() == 0 {
		return
	}
	wsConn, ok := conn.Session().Load("wsConn")
	if !ok {
		_ = conn.WriteClose(1000, nil)
		return
	}
	wsConn.(*WsConn).requestManager.handleResponse(message)
}

// OnClose handles WebSocket connection closures and triggers system down status after delay.
func (h *Handler) OnClose(conn *gws.Conn, err error) {
	wsConn, ok := conn.Session().Load("wsConn")
	if !ok {
		return
	}
	wsConn.(*WsConn).conn = nil
	// wait 5 seconds to allow reconnection before setting system down
	// use a weak pointer to avoid keeping references if the system is removed
	go func(downChan weak.Pointer[chan struct{}]) {
		time.Sleep(5 * time.Second)
		downChanValue := downChan.Value()
		if downChanValue != nil {
			*downChanValue <- struct{}{}
		}
	}(weak.Make(&wsConn.(*WsConn).DownChan))
}

// Close terminates the WebSocket connection gracefully.
func (ws *WsConn) Close(msg []byte) {
	if ws.IsConnected() {
		ws.conn.WriteClose(1000, msg)
	}
	if ws.requestManager != nil {
		ws.requestManager.Close()
	}
}

// Ping sends a ping frame to keep the connection alive.
func (ws *WsConn) Ping() error {
	ws.conn.SetDeadline(time.Now().Add(deadline))
	return ws.conn.WritePing(nil)
}

// sendMessage encodes data to CBOR and sends it as a binary message to the agent.
// This is kept for backwards compatibility but new actions should use RequestManager.
func (ws *WsConn) sendMessage(data common.HubRequest[any]) error {
	if ws.conn == nil {
		return gws.ErrConnClosed
	}
	bytes, err := cbor.Marshal(data)
	if err != nil {
		return err
	}
	return ws.conn.WriteMessage(gws.OpcodeBinary, bytes)
}

// handleAgentRequest processes a request to the agent, handling both legacy and new formats.
func (ws *WsConn) handleAgentRequest(req *PendingRequest, handler ResponseHandler) error {
	// Wait for response
	select {
	case message := <-req.ResponseCh:
		defer message.Close()
		// Cancel request context to stop timeout watcher promptly
		defer req.Cancel()
		data := message.Data.Bytes()

		// Legacy format - unmarshal directly
		if ws.agentVersion.LT(beszel.MinVersionAgentResponse) {
			return handler.HandleLegacy(data)
		}

		// New format with AgentResponse wrapper
		var agentResponse common.AgentResponse
		if err := cbor.Unmarshal(data, &agentResponse); err != nil {
			return err
		}
		if agentResponse.Error != "" {
			return errors.New(agentResponse.Error)
		}
		return handler.Handle(agentResponse)

	case <-req.Context.Done():
		return req.Context.Err()
	}
}

// IsConnected returns true if the WebSocket connection is active.
func (ws *WsConn) IsConnected() bool {
	return ws.conn != nil
}
