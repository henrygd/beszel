//go:build testing

package agent

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/stretchr/testify/assert"
)

// MockHandler for testing
type MockHandler struct {
	requiresVerification bool
	description          string
	handleFunc           func(ctx *HandlerContext) error
}

func (m *MockHandler) Handle(ctx *HandlerContext) error {
	if m.handleFunc != nil {
		return m.handleFunc(ctx)
	}
	return nil
}

func (m *MockHandler) RequiresVerification() bool {
	return m.requiresVerification
}

// TestHandlerRegistry tests the handler registry functionality
func TestHandlerRegistry(t *testing.T) {
	t.Run("default registration", func(t *testing.T) {
		registry := NewHandlerRegistry()

		// Check default handlers are registered
		getDataHandler, exists := registry.GetHandler(common.GetData)
		assert.True(t, exists)
		assert.IsType(t, &GetDataHandler{}, getDataHandler)

		fingerprintHandler, exists := registry.GetHandler(common.CheckFingerprint)
		assert.True(t, exists)
		assert.IsType(t, &CheckFingerprintHandler{}, fingerprintHandler)
	})

	t.Run("custom handler registration", func(t *testing.T) {
		registry := NewHandlerRegistry()
		mockHandler := &MockHandler{
			requiresVerification: true,
			description:          "Test handler",
		}

		// Register a custom handler for a mock action
		const mockAction common.WebSocketAction = 99
		registry.Register(mockAction, mockHandler)

		// Verify registration
		handler, exists := registry.GetHandler(mockAction)
		assert.True(t, exists)
		assert.Equal(t, mockHandler, handler)
	})

	t.Run("unknown action", func(t *testing.T) {
		registry := NewHandlerRegistry()
		ctx := &HandlerContext{
			Request: &common.HubRequest[cbor.RawMessage]{
				Action: common.WebSocketAction(255), // Unknown action
			},
			HubVerified: true,
		}

		err := registry.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown action: 255")
	})

	t.Run("verification required", func(t *testing.T) {
		registry := NewHandlerRegistry()
		ctx := &HandlerContext{
			Request: &common.HubRequest[cbor.RawMessage]{
				Action: common.GetData, // Requires verification
			},
			HubVerified: false, // Not verified
		}

		err := registry.Handle(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hub not verified")
	})
}

// TestCheckFingerprintHandler tests the CheckFingerprint handler
func TestCheckFingerprintHandler(t *testing.T) {
	handler := &CheckFingerprintHandler{}

	t.Run("handle with invalid data", func(t *testing.T) {
		client := &WebSocketClient{}
		ctx := &HandlerContext{
			Client:      client,
			HubVerified: false,
			Request: &common.HubRequest[cbor.RawMessage]{
				Action: common.CheckFingerprint,
				Data:   cbor.RawMessage{}, // Empty/invalid data
			},
		}

		// Should fail to decode the fingerprint request
		err := handler.Handle(ctx)
		assert.Error(t, err)
	})
}
