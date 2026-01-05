package ws

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/lxzan/gws"
)

// RequestID uniquely identifies a request
type RequestID uint32

// PendingRequest tracks an in-flight request
type PendingRequest struct {
	ID         RequestID
	ResponseCh chan *gws.Message
	Context    context.Context
	Cancel     context.CancelFunc
	CreatedAt  time.Time
}

// RequestManager handles concurrent requests to an agent
type RequestManager struct {
	sync.RWMutex
	conn        *gws.Conn
	pendingReqs map[RequestID]*PendingRequest
	nextID      atomic.Uint32
}

// NewRequestManager creates a new request manager for a WebSocket connection
func NewRequestManager(conn *gws.Conn) *RequestManager {
	rm := &RequestManager{
		conn:        conn,
		pendingReqs: make(map[RequestID]*PendingRequest),
	}
	return rm
}

// SendRequest sends a request and returns a channel for the response
func (rm *RequestManager) SendRequest(ctx context.Context, action common.WebSocketAction, data any) (*PendingRequest, error) {
	reqID := RequestID(rm.nextID.Add(1))

	// Respect any caller-provided deadline. If none is set, apply a reasonable default
	// so pending requests don't live forever if the agent never responds.
	reqCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		reqCtx, cancel = context.WithCancel(ctx)
	} else {
		reqCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	}

	req := &PendingRequest{
		ID:         reqID,
		ResponseCh: make(chan *gws.Message, 1),
		Context:    reqCtx,
		Cancel:     cancel,
		CreatedAt:  time.Now(),
	}

	rm.Lock()
	rm.pendingReqs[reqID] = req
	rm.Unlock()

	hubReq := common.HubRequest[any]{
		Id:     (*uint32)(&reqID),
		Action: action,
		Data:   data,
	}

	// Send the request
	if err := rm.sendMessage(hubReq); err != nil {
		rm.cancelRequest(reqID)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Start cleanup watcher for timeout/cancellation
	go rm.cleanupRequest(req)

	return req, nil
}

// sendMessage encodes and sends a message over WebSocket
func (rm *RequestManager) sendMessage(data any) error {
	if rm.conn == nil {
		return gws.ErrConnClosed
	}

	bytes, err := cbor.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	return rm.conn.WriteMessage(gws.OpcodeBinary, bytes)
}

// handleResponse processes a single response message
func (rm *RequestManager) handleResponse(message *gws.Message) {
	var response common.AgentResponse
	if err := cbor.Unmarshal(message.Data.Bytes(), &response); err != nil {
		// Legacy response without ID - route to first pending request of any type
		rm.routeLegacyResponse(message)
		return
	}

	if response.Id == nil {
		rm.routeLegacyResponse(message)
		return
	}

	reqID := RequestID(*response.Id)

	rm.RLock()
	req, exists := rm.pendingReqs[reqID]
	rm.RUnlock()

	if !exists {
		// Request not found (might have timed out) - close the message
		message.Close()
		return
	}

	select {
	case req.ResponseCh <- message:
		// Message successfully delivered - the receiver will close it
		rm.deleteRequest(reqID)
	case <-req.Context.Done():
		// Request was cancelled/timed out - close the message
		message.Close()
	}
}

// routeLegacyResponse handles responses that don't have request IDs (backwards compatibility)
func (rm *RequestManager) routeLegacyResponse(message *gws.Message) {
	// Snapshot the oldest pending request without holding the lock during send
	rm.RLock()
	var oldestReq *PendingRequest
	for _, req := range rm.pendingReqs {
		if oldestReq == nil || req.CreatedAt.Before(oldestReq.CreatedAt) {
			oldestReq = req
		}
	}
	rm.RUnlock()

	if oldestReq != nil {
		select {
		case oldestReq.ResponseCh <- message:
			// Message successfully delivered - the receiver will close it
			rm.deleteRequest(oldestReq.ID)
		case <-oldestReq.Context.Done():
			// Request was cancelled - close the message
			message.Close()
		}
	} else {
		// No pending requests - close the message
		message.Close()
	}
}

// cleanupRequest handles request timeout and cleanup
func (rm *RequestManager) cleanupRequest(req *PendingRequest) {
	<-req.Context.Done()
	rm.cancelRequest(req.ID)
}

// cancelRequest removes a request and cancels its context
func (rm *RequestManager) cancelRequest(reqID RequestID) {
	rm.Lock()
	defer rm.Unlock()

	if req, exists := rm.pendingReqs[reqID]; exists {
		req.Cancel()
		delete(rm.pendingReqs, reqID)
	}
}

// deleteRequest removes a request from the pending map without cancelling its context.
func (rm *RequestManager) deleteRequest(reqID RequestID) {
	rm.Lock()
	defer rm.Unlock()
	delete(rm.pendingReqs, reqID)
}

// Close shuts down the request manager
func (rm *RequestManager) Close() {
	rm.Lock()
	defer rm.Unlock()

	// Cancel all pending requests
	for _, req := range rm.pendingReqs {
		req.Cancel()
	}
	rm.pendingReqs = make(map[RequestID]*PendingRequest)
}
