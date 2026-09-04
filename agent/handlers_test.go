//go:build testing

package agent

import (
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/agent/zfs"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/stretchr/testify/assert"
)

// MockHandler for testing
type MockHandler struct {
	requiresVerification bool
	description          string
	handleFunc           func(ctx *HandlerContext) error
}

func TestNewAgentResponseSmartData(t *testing.T) {
	response := newAgentResponse(smart.SmartDataResponse{
		Data: map[string]smart.SmartData{
			"AAA": {SerialNumber: "AAA"},
		},
		Complete: true,
	}, nil)

	assert.Equal(t, "AAA", response.SmartData["AAA"].SerialNumber)
	assert.True(t, response.SmartComplete)
}

func TestGetZfsDataHandlerForceRefresh(t *testing.T) {
	poolCalls := 0
	zm := &ZfsManager{detailInterval: time.Hour}
	zm.poolStatsFn = func() ([]zfs.PoolStat, error) {
		poolCalls++
		return []zfs.PoolStat{{Name: "tank", Alloc: uint64(poolCalls)}}, nil
	}
	zm.poolStatusesFn = func() ([]zfs.PoolStatus, error) { return nil, nil }
	zm.datasetsFn = func() ([]zfs.Dataset, error) { return nil, nil }
	zm.GetDetail(false)

	requestData, err := cbor.Marshal(common.ZfsDataRequest{Force: true})
	assert.NoError(t, err)
	ctx := &HandlerContext{
		Agent: &Agent{zfsManager: zm},
		Request: &common.HubRequest[cbor.RawMessage]{
			Action: common.GetZfsData,
			Data:   requestData,
		},
		SendResponse: func(any, *uint32) error { return nil },
	}

	assert.NoError(t, (&GetZfsDataHandler{}).Handle(ctx))
	assert.Equal(t, 2, poolCalls)
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
