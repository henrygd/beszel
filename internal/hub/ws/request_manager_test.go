//go:build testing

package ws

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRequestManager_BasicFunctionality tests the request manager without mocking gws.Conn
func TestRequestManager_BasicFunctionality(t *testing.T) {
	// We'll test the core logic without mocking the connection
	// since the gws.Conn interface is complex to mock properly

	t.Run("request ID generation", func(t *testing.T) {
		// Test that request IDs are generated sequentially and uniquely
		rm := &RequestManager{}

		// Simulate multiple ID generations
		id1 := rm.nextID.Add(1)
		id2 := rm.nextID.Add(1)
		id3 := rm.nextID.Add(1)

		assert.NotEqual(t, id1, id2)
		assert.NotEqual(t, id2, id3)
		assert.Greater(t, id2, id1)
		assert.Greater(t, id3, id2)
	})

	t.Run("pending request tracking", func(t *testing.T) {
		rm := &RequestManager{
			pendingReqs: make(map[RequestID]*PendingRequest),
		}

		// Initially no pending requests
		assert.Equal(t, 0, rm.GetPendingCount())

		// Add some fake pending requests
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req1 := &PendingRequest{
			ID:      RequestID(1),
			Context: ctx,
			Cancel:  cancel,
		}
		req2 := &PendingRequest{
			ID:      RequestID(2),
			Context: ctx,
			Cancel:  cancel,
		}

		rm.pendingReqs[req1.ID] = req1
		rm.pendingReqs[req2.ID] = req2

		assert.Equal(t, 2, rm.GetPendingCount())

		// Remove one
		delete(rm.pendingReqs, req1.ID)
		assert.Equal(t, 1, rm.GetPendingCount())

		// Remove all
		delete(rm.pendingReqs, req2.ID)
		assert.Equal(t, 0, rm.GetPendingCount())
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Wait for context to timeout
		<-ctx.Done()

		// Verify context was cancelled
		assert.Equal(t, context.DeadlineExceeded, ctx.Err())
	})
}
