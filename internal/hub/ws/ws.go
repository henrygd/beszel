package ws

import (
	"errors"
	"time"
	"weak"

	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
	"golang.org/x/crypto/ssh"
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
	conn         *gws.Conn
	responseChan chan *gws.Message
	DownChan     chan struct{}
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
	upgrader = gws.NewUpgrader(handler, &gws.ServerOption{})
	return upgrader
}

// NewWsConnection creates a new WebSocket connection wrapper.
func NewWsConnection(conn *gws.Conn) *WsConn {
	return &WsConn{
		conn:         conn,
		responseChan: make(chan *gws.Message, 1),
		DownChan:     make(chan struct{}, 1),
	}
}

// OnOpen sets a deadline for the WebSocket connection.
func (h *Handler) OnOpen(conn *gws.Conn) {
	conn.SetDeadline(time.Now().Add(deadline))
}

// OnMessage routes incoming WebSocket messages to the response channel.
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
	select {
	case wsConn.(*WsConn).responseChan <- message:
	default:
		// close if the connection is not expecting a response
		wsConn.(*WsConn).Close(nil)
	}
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
}

// Ping sends a ping frame to keep the connection alive.
func (ws *WsConn) Ping() error {
	ws.conn.SetDeadline(time.Now().Add(deadline))
	return ws.conn.WritePing(nil)
}

// sendMessage encodes data to CBOR and sends it as a binary message to the agent.
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

// RequestSystemData requests system metrics from the agent and unmarshals the response.
func (ws *WsConn) RequestSystemData(data *system.CombinedData) error {
	var message *gws.Message

	ws.sendMessage(common.HubRequest[any]{
		Action: common.GetData,
	})
	select {
	case <-time.After(10 * time.Second):
		ws.Close(nil)
		return gws.ErrConnClosed
	case message = <-ws.responseChan:
	}
	defer message.Close()
	return cbor.Unmarshal(message.Data.Bytes(), data)
}

// GetFingerprint authenticates with the agent using SSH signature and returns the agent's fingerprint.
func (ws *WsConn) GetFingerprint(token string, signer ssh.Signer, needSysInfo bool) (common.FingerprintResponse, error) {
	var clientFingerprint common.FingerprintResponse
	challenge := []byte(token)

	signature, err := signer.Sign(nil, challenge)
	if err != nil {
		return clientFingerprint, err
	}

	err = ws.sendMessage(common.HubRequest[any]{
		Action: common.CheckFingerprint,
		Data: common.FingerprintRequest{
			Signature:   signature.Blob,
			NeedSysInfo: needSysInfo,
		},
	})
	if err != nil {
		return clientFingerprint, err
	}

	var message *gws.Message
	select {
	case message = <-ws.responseChan:
	case <-time.After(10 * time.Second):
		return clientFingerprint, errors.New("request expired")
	}
	defer message.Close()

	err = cbor.Unmarshal(message.Data.Bytes(), &clientFingerprint)
	return clientFingerprint, err
}

// IsConnected returns true if the WebSocket connection is active.
func (ws *WsConn) IsConnected() bool {
	return ws.conn != nil
}
