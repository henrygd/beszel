//go:build testing

package ws

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// TestGetUpgrader tests the singleton upgrader
func TestGetUpgrader(t *testing.T) {
	// Reset the global upgrader to test singleton behavior
	upgrader = nil

	// First call should create the upgrader
	upgrader1 := GetUpgrader()
	assert.NotNil(t, upgrader1, "Upgrader should not be nil")

	// Second call should return the same instance
	upgrader2 := GetUpgrader()
	assert.Same(t, upgrader1, upgrader2, "Should return the same upgrader instance")

	// Verify it's properly configured
	assert.NotNil(t, upgrader1, "Upgrader should be configured")
}

// TestNewWsConnection tests WebSocket connection creation
func TestNewWsConnection(t *testing.T) {
	// We can't easily mock gws.Conn, so we'll pass nil and test the structure
	wsConn := NewWsConnection(nil, semver.MustParse("0.12.10"))

	assert.NotNil(t, wsConn, "WebSocket connection should not be nil")
	assert.Nil(t, wsConn.conn, "Connection should be nil as passed")
	assert.NotNil(t, wsConn.requestManager, "Request manager should be initialized")
	assert.NotNil(t, wsConn.DownChan, "Down channel should be initialized")
	assert.Equal(t, 1, cap(wsConn.DownChan), "Down channel should have capacity of 1")
}

// TestWsConn_IsConnected tests the connection status check
func TestWsConn_IsConnected(t *testing.T) {
	// Test with nil connection
	wsConn := NewWsConnection(nil, semver.MustParse("0.12.10"))
	assert.False(t, wsConn.IsConnected(), "Should not be connected when conn is nil")
}

// TestWsConn_Close tests the connection closing with nil connection
func TestWsConn_Close(t *testing.T) {
	wsConn := NewWsConnection(nil, semver.MustParse("0.12.10"))

	// Should handle nil connection gracefully
	assert.NotPanics(t, func() {
		wsConn.Close([]byte("test message"))
	}, "Should not panic when closing nil connection")
}

// TestWsConn_SendMessage_CBOR tests CBOR encoding in sendMessage
func TestWsConn_SendMessage_CBOR(t *testing.T) {
	wsConn := NewWsConnection(nil, semver.MustParse("0.12.10"))

	testData := common.HubRequest[any]{
		Action: common.GetData,
		Data:   "test data",
	}

	// This will fail because conn is nil, but we can test the CBOR encoding logic
	// by checking that the function properly encodes to CBOR before failing
	err := wsConn.sendMessage(testData)
	assert.Error(t, err, "Should error with nil connection")

	// Test CBOR encoding separately
	bytes, err := cbor.Marshal(testData)
	assert.NoError(t, err, "Should encode to CBOR successfully")

	// Verify we can decode it back
	var decodedData common.HubRequest[any]
	err = cbor.Unmarshal(bytes, &decodedData)
	assert.NoError(t, err, "Should decode from CBOR successfully")
	assert.Equal(t, testData.Action, decodedData.Action, "Action should match")
}

// TestWsConn_GetFingerprint_SignatureGeneration tests signature creation logic
func TestWsConn_GetFingerprint_SignatureGeneration(t *testing.T) {
	// Generate test key pair
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(privKey)
	require.NoError(t, err)

	token := "test-token"

	// This will timeout since conn is nil, but we can verify the signature logic
	// We can't test the full flow, but we can test that the signature is created properly
	challenge := []byte(token)
	signature, err := signer.Sign(nil, challenge)
	assert.NoError(t, err, "Should create signature successfully")
	assert.NotEmpty(t, signature.Blob, "Signature blob should not be empty")
	assert.Equal(t, signer.PublicKey().Type(), signature.Format, "Signature format should match key type")

	// Test the fingerprint request structure
	fpRequest := common.FingerprintRequest{
		Signature:   signature.Blob,
		NeedSysInfo: true,
	}

	// Test CBOR encoding of fingerprint request
	fpData, err := cbor.Marshal(fpRequest)
	assert.NoError(t, err, "Should encode fingerprint request to CBOR")

	var decodedFpRequest common.FingerprintRequest
	err = cbor.Unmarshal(fpData, &decodedFpRequest)
	assert.NoError(t, err, "Should decode fingerprint request from CBOR")
	assert.Equal(t, fpRequest.Signature, decodedFpRequest.Signature, "Signature should match")
	assert.Equal(t, fpRequest.NeedSysInfo, decodedFpRequest.NeedSysInfo, "NeedSysInfo should match")

	// Test the full hub request structure
	hubRequest := common.HubRequest[any]{
		Action: common.CheckFingerprint,
		Data:   fpRequest,
	}

	hubData, err := cbor.Marshal(hubRequest)
	assert.NoError(t, err, "Should encode hub request to CBOR")

	var decodedHubRequest common.HubRequest[cbor.RawMessage]
	err = cbor.Unmarshal(hubData, &decodedHubRequest)
	assert.NoError(t, err, "Should decode hub request from CBOR")
	assert.Equal(t, common.CheckFingerprint, decodedHubRequest.Action, "Action should be CheckFingerprint")
}

// TestWsConn_RequestSystemData_RequestFormat tests system data request format
func TestWsConn_RequestSystemData_RequestFormat(t *testing.T) {
	// Test the request format that would be sent
	request := common.HubRequest[any]{
		Action: common.GetData,
	}

	// Test CBOR encoding
	data, err := cbor.Marshal(request)
	assert.NoError(t, err, "Should encode request to CBOR")

	// Test decoding
	var decodedRequest common.HubRequest[any]
	err = cbor.Unmarshal(data, &decodedRequest)
	assert.NoError(t, err, "Should decode request from CBOR")
	assert.Equal(t, common.GetData, decodedRequest.Action, "Should have GetData action")
}

// TestFingerprintRecord tests the FingerprintRecord struct
func TestFingerprintRecord(t *testing.T) {
	record := FingerprintRecord{
		Id:          "test-id",
		SystemId:    "system-123",
		Fingerprint: "test-fingerprint",
		Token:       "test-token",
	}

	assert.Equal(t, "test-id", record.Id)
	assert.Equal(t, "system-123", record.SystemId)
	assert.Equal(t, "test-fingerprint", record.Fingerprint)
	assert.Equal(t, "test-token", record.Token)
}

// TestDeadlineConstant tests that the deadline constant is reasonable
func TestDeadlineConstant(t *testing.T) {
	assert.Equal(t, 70*time.Second, deadline, "Deadline should be 70 seconds")
}

// TestCommonActions tests that the common actions are properly defined
func TestCommonActions(t *testing.T) {
	// Test that the actions we use exist and have expected values
	assert.Equal(t, common.WebSocketAction(0), common.GetData, "GetData should be action 0")
	assert.Equal(t, common.WebSocketAction(1), common.CheckFingerprint, "CheckFingerprint should be action 1")
	assert.Equal(t, common.WebSocketAction(2), common.GetContainerLogs, "GetLogs should be action 2")
}

func TestFingerprintHandler(t *testing.T) {
	var result common.FingerprintResponse
	h := &fingerprintHandler{result: &result}

	resp := common.AgentResponse{Fingerprint: &common.FingerprintResponse{
		Fingerprint: "test-fingerprint",
		Hostname:    "test-host",
	}}
	err := h.Handle(resp)
	assert.NoError(t, err)
	assert.Equal(t, "test-fingerprint", result.Fingerprint)
	assert.Equal(t, "test-host", result.Hostname)
}

// TestHandler tests that we can create a Handler
func TestHandler(t *testing.T) {
	handler := &Handler{}
	assert.NotNil(t, handler, "Handler should be created successfully")

	// The Handler embeds gws.BuiltinEventHandler, so it should have the embedded type
	assert.NotNil(t, handler.BuiltinEventHandler, "Should have embedded BuiltinEventHandler")
}

// TestWsConnChannelBehavior tests channel behavior without WebSocket connections
func TestWsConnChannelBehavior(t *testing.T) {
	wsConn := NewWsConnection(nil, semver.MustParse("0.12.10"))

	// Test that channels are properly initialized and can be used
	select {
	case wsConn.DownChan <- struct{}{}:
		// Should be able to write to channel
	default:
		t.Error("Should be able to write to DownChan")
	}

	// Test reading from DownChan
	select {
	case <-wsConn.DownChan:
		// Should be able to read from channel
	case <-time.After(10 * time.Millisecond):
		t.Error("Should be able to read from DownChan")
	}

	// Request manager should have no pending requests initially
	assert.Equal(t, 0, wsConn.requestManager.GetPendingCount(), "Should have no pending requests initially")
}
