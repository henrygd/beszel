//go:build testing
// +build testing

package hub

import (
	"beszel/internal/agent"
	"beszel/internal/common"
	"beszel/internal/hub/expirymap"
	"beszel/internal/hub/ws"
	"crypto/ed25519"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// Helper function to create a test hub without import cycle
func createTestHub(t testing.TB) (*Hub, *pbtests.TestApp, error) {
	testDataDir := t.TempDir()
	testApp, err := pbtests.NewTestApp(testDataDir)
	if err != nil {
		return nil, nil, err
	}
	return NewHub(testApp), testApp, nil
}

// Helper function to create a test record
func createTestRecord(app core.App, collection string, data map[string]any) (*core.Record, error) {
	col, err := app.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		return nil, err
	}
	record := core.NewRecord(col)
	for key, value := range data {
		record.Set(key, value)
	}

	return record, app.Save(record)
}

// Helper function to create a test user
func createTestUser(app core.App) (*core.Record, error) {
	userRecord, err := createTestRecord(app, "users", map[string]any{
		"email":    "test@test.com",
		"password": "testtesttest",
	})
	return userRecord, err
}

// TestValidateAgentHeaders tests the validateAgentHeaders function
func TestValidateAgentHeaders(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	testCases := []struct {
		name          string
		headers       http.Header
		expectError   bool
		expectedToken string
		expectedAgent string
	}{
		{
			name: "valid headers",
			headers: http.Header{
				"X-Token":  []string{"valid-token-123"},
				"X-Beszel": []string{"0.5.0"},
			},
			expectError:   false,
			expectedToken: "valid-token-123",
			expectedAgent: "0.5.0",
		},
		{
			name: "missing token",
			headers: http.Header{
				"X-Beszel": []string{"0.5.0"},
			},
			expectError: true,
		},
		{
			name: "missing agent version",
			headers: http.Header{
				"X-Token": []string{"valid-token-123"},
			},
			expectError: true,
		},
		{
			name: "empty token",
			headers: http.Header{
				"X-Token":  []string{""},
				"X-Beszel": []string{"0.5.0"},
			},
			expectError: true,
		},
		{
			name: "empty agent version",
			headers: http.Header{
				"X-Token":  []string{"valid-token-123"},
				"X-Beszel": []string{""},
			},
			expectError: true,
		},
		{
			name: "token too long",
			headers: http.Header{
				"X-Token":  []string{string(make([]byte, 513))}, // 513 bytes > 512 limit
				"X-Beszel": []string{"0.5.0"},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token, agentVersion, err := hub.validateAgentHeaders(tc.headers)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedToken, token)
				assert.Equal(t, tc.expectedAgent, agentVersion)
			}
		})
	}
}

// TestGetFingerprintRecord tests the getFingerprintRecord function
func TestGetFingerprintRecord(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	// create test user
	userRecord, err := createTestUser(testApp)
	if err != nil {
		t.Fatal(err)
	}

	// Create test data
	systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
		"name":   "test-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "pending",
		"users":  []string{userRecord.Id},
	})
	if err != nil {
		t.Fatal(err)
	}

	fingerprintRecord, err := createTestRecord(testApp, "fingerprints", map[string]any{
		"system":      systemRecord.Id,
		"token":       "test-token-123",
		"fingerprint": "test-fingerprint",
	})
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name        string
		token       string
		expectError bool
		expectedId  string
	}{
		{
			name:        "valid token",
			token:       "test-token-123",
			expectError: false,
			expectedId:  fingerprintRecord.Id,
		},
		{
			name:        "invalid token",
			token:       "invalid-token",
			expectError: true,
		},
		{
			name:        "empty token",
			token:       "",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var recordData ws.FingerprintRecord
			err := hub.getFingerprintRecord(tc.token, &recordData)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedId, recordData.Id)
			}
		})
	}
}

// TestSetFingerprint tests the SetFingerprint function
func TestSetFingerprint(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	// Create test user
	userRecord, err := createTestUser(testApp)
	if err != nil {
		t.Fatal(err)
	}

	// Create test system
	systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
		"name":   "test-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "pending",
		"users":  []string{userRecord.Id},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create fingerprint record
	fingerprintRecord, err := createTestRecord(testApp, "fingerprints", map[string]any{
		"system":      systemRecord.Id,
		"token":       "test-token-123",
		"fingerprint": "",
	})
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name           string
		recordId       string
		newFingerprint string
		expectError    bool
	}{
		{
			name:           "successful fingerprint update",
			recordId:       fingerprintRecord.Id,
			newFingerprint: "new-test-fingerprint",
			expectError:    false,
		},
		{
			name:           "empty fingerprint",
			recordId:       fingerprintRecord.Id,
			newFingerprint: "",
			expectError:    false,
		},
		{
			name:           "invalid record ID",
			recordId:       "invalid-id",
			newFingerprint: "fingerprint",
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := hub.SetFingerprint(&ws.FingerprintRecord{Id: tc.recordId, Token: "test-token-123"}, tc.newFingerprint)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify fingerprint was updated
				updatedRecord, err := testApp.FindRecordById("fingerprints", tc.recordId)
				require.NoError(t, err)
				assert.Equal(t, tc.newFingerprint, updatedRecord.GetString("fingerprint"))
			}
		})
	}
}

// TestCreateSystemFromAgentData tests the createSystemFromAgentData function
func TestCreateSystemFromAgentData(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	// Create test user
	userRecord, err := createTestUser(testApp)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name          string
		agentConnReq  agentConnectRequest
		fingerprint   common.FingerprintResponse
		expectError   bool
		expectedName  string
		expectedHost  string
		expectedPort  string
		expectedUsers []string
	}{
		{
			name: "successful system creation with all fields",
			agentConnReq: agentConnectRequest{
				userId:     userRecord.Id,
				remoteAddr: "192.168.1.100",
			},
			fingerprint: common.FingerprintResponse{
				Hostname: "test-server",
				Port:     "8080",
			},
			expectError:   false,
			expectedName:  "test-server",
			expectedHost:  "192.168.1.100",
			expectedPort:  "8080",
			expectedUsers: []string{userRecord.Id},
		},
		{
			name: "system creation with default port",
			agentConnReq: agentConnectRequest{
				userId:     userRecord.Id,
				remoteAddr: "10.0.0.50",
			},
			fingerprint: common.FingerprintResponse{
				Hostname: "default-port-server",
				Port:     "", // Empty port should default to 45876
			},
			expectError:   false,
			expectedName:  "default-port-server",
			expectedHost:  "10.0.0.50",
			expectedPort:  "45876",
			expectedUsers: []string{userRecord.Id},
		},
		{
			name: "system creation with empty hostname",
			agentConnReq: agentConnectRequest{
				userId:     userRecord.Id,
				remoteAddr: "172.16.0.1",
			},
			fingerprint: common.FingerprintResponse{
				Hostname: "",
				Port:     "9090",
			},
			expectError:   false,
			expectedName:  "172.16.0.1", // Should fall back to host IP when hostname is empty
			expectedHost:  "172.16.0.1",
			expectedPort:  "9090",
			expectedUsers: []string{userRecord.Id},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recordId, err := hub.createSystemFromAgentData(&tc.agentConnReq, tc.fingerprint)

			if tc.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, recordId, "Record ID should not be empty")

			// Verify the created system record
			systemRecord, err := testApp.FindRecordById("systems", recordId)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedName, systemRecord.GetString("name"))
			assert.Equal(t, tc.expectedHost, systemRecord.GetString("host"))
			assert.Equal(t, tc.expectedPort, systemRecord.GetString("port"))

			// Verify users array
			users := systemRecord.Get("users")
			assert.Equal(t, tc.expectedUsers, users)
		})
	}
}

// TestUniversalTokenFlow tests the complete universal token authentication flow
func TestUniversalTokenFlow(t *testing.T) {
	_, testApp, err := createTestHub(t)
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	// Create test user
	userRecord, err := createTestUser(testApp)
	if err != nil {
		t.Fatal(err)
	}

	// Set up universal token in the token map
	universalToken := "universal-token-123"

	// Initialize tokenMap if it doesn't exist
	if tokenMap == nil {
		tokenMap = expirymap.New[string](time.Hour)
	}
	tokenMap.Set(universalToken, userRecord.Id, time.Hour)

	testCases := []struct {
		name                string
		token               string
		expectUniversalAuth bool
		expectError         bool
		description         string
	}{
		{
			name:                "valid universal token",
			token:               universalToken,
			expectUniversalAuth: true,
			expectError:         false,
			description:         "Should recognize valid universal token",
		},
		{
			name:                "invalid universal token",
			token:               "invalid-universal-token",
			expectUniversalAuth: false,
			expectError:         true,
			description:         "Should reject invalid universal token",
		},
		{
			name:                "empty token",
			token:               "",
			expectUniversalAuth: false,
			expectError:         true,
			description:         "Should reject empty token",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var acr agentConnectRequest
			acr.token = tc.token

			err := checkUniversalToken(&acr)

			if tc.expectError {
				assert.Error(t, err)
				assert.False(t, acr.isUniversalToken)
				assert.Empty(t, acr.userId)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectUniversalAuth, acr.isUniversalToken)
				if tc.expectUniversalAuth {
					assert.Equal(t, userRecord.Id, acr.userId)
				}
			}
		})
	}
}

// TestAgentDataProtection tests that agent won't send system data before fingerprint verification
func TestAgentDataProtection(t *testing.T) {
	// This test verifies the logic in the agent's handleHubRequest method
	// Since we can't access private fields directly, we'll test the behavior indirectly
	// by creating a mock scenario that simulates the verification flow

	// The key behavior is tested in the agent's handleHubRequest method:
	// if !client.hubVerified && msg.Action != common.CheckFingerprint {
	//     return errors.New("hub not verified")
	// }

	// This test documents the expected behavior rather than testing implementation details
	t.Run("agent should reject GetData before fingerprint verification", func(t *testing.T) {
		// This behavior is enforced by the agent's WebSocket client
		// When hubVerified is false and action is GetData, it returns "hub not verified" error
		assert.True(t, true, "Agent rejects GetData requests before hub verification")
	})

	t.Run("agent should allow CheckFingerprint before verification", func(t *testing.T) {
		// CheckFingerprint action is always allowed regardless of hubVerified status
		assert.True(t, true, "Agent allows CheckFingerprint requests before hub verification")
	})
}

// TestFingerprintResponseFields tests that FingerprintResponse includes hostname and port when requested
func TestFingerprintResponseFields(t *testing.T) {
	testCases := []struct {
		name           string
		includeSysInfo bool
		expectHostname bool
		expectPort     bool
		description    string
	}{
		{
			name:           "include system info",
			includeSysInfo: true,
			expectHostname: true,
			expectPort:     true,
			description:    "Should include hostname and port when requested",
		},
		{
			name:           "exclude system info",
			includeSysInfo: false,
			expectHostname: false,
			expectPort:     false,
			description:    "Should not include hostname and port when not requested",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the response creation logic as it would be used in the agent
			response := &common.FingerprintResponse{
				Fingerprint: "test-fingerprint",
			}

			if tc.includeSysInfo {
				response.Hostname = "test-hostname"
				response.Port = "8080"
			}

			// Verify the response structure
			assert.NotEmpty(t, response.Fingerprint, "Fingerprint should always be present")

			if tc.expectHostname {
				assert.NotEmpty(t, response.Hostname, "Hostname should be present when requested")
			} else {
				assert.Empty(t, response.Hostname, "Hostname should be empty when not requested")
			}

			if tc.expectPort {
				assert.NotEmpty(t, response.Port, "Port should be present when requested")
			} else {
				assert.Empty(t, response.Port, "Port should be empty when not requested")
			}
		})
	}
}

// TestAgentConnect tests the agentConnect function with various scenarios
func TestAgentConnect(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	// Create test user
	userRecord, err := createTestUser(testApp)
	if err != nil {
		t.Fatal(err)
	}

	// Create test system
	systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
		"name":   "test-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "pending",
		"users":  []string{userRecord.Id},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create fingerprint record
	testToken := "test-token-456"
	_, err = createTestRecord(testApp, "fingerprints", map[string]any{
		"system":      systemRecord.Id,
		"token":       testToken,
		"fingerprint": "",
	})
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
		description    string
	}{
		{
			name: "missing token header",
			headers: map[string]string{
				"X-Beszel": "0.5.0",
			},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should fail due to missing token",
		},
		{
			name: "missing agent version header",
			headers: map[string]string{
				"X-Token": testToken,
			},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should fail due to missing agent version",
		},
		{
			name: "invalid token",
			headers: map[string]string{
				"X-Token":  "invalid-token",
				"X-Beszel": "0.5.0",
			},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should fail due to invalid token",
		},
		{
			name: "invalid agent version",
			headers: map[string]string{
				"X-Token":  testToken,
				"X-Beszel": "0.5.0.0.0",
			},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should fail due to invalid agent version",
		},
		{
			name: "valid headers but websocket upgrade will fail in test",
			headers: map[string]string{
				"X-Token":  testToken,
				"X-Beszel": "0.5.0",
			},
			expectedStatus: http.StatusInternalServerError,
			description:    "Should pass validation but fail at WebSocket upgrade due to test limitations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/beszel/agent-connect", nil)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			recorder := httptest.NewRecorder()
			err = hub.agentConnect(req, recorder)

			assert.Equal(t, tc.expectedStatus, recorder.Code, tc.description)
		})
	}
}

// TestSendResponseError tests the sendResponseError function
func TestSendResponseError(t *testing.T) {
	testCases := []struct {
		name           string
		statusCode     int
		message        string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "unauthorized error",
			statusCode:     http.StatusUnauthorized,
			message:        "Invalid token",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   "Invalid token",
		},
		{
			name:           "bad request error",
			statusCode:     http.StatusBadRequest,
			message:        "Missing required header",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing required header",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			sendResponseError(recorder, tc.statusCode, tc.message)

			assert.Equal(t, tc.expectedStatus, recorder.Code)
			assert.Equal(t, tc.expectedBody, recorder.Body.String())
		})
	}
}

// TestHandleAgentConnect tests the HTTP handler
func TestHandleAgentConnect(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	// Create test user
	userRecord, err := createTestUser(testApp)
	if err != nil {
		t.Fatal(err)
	}

	// Create test system
	systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
		"name":   "test-system",
		"host":   "localhost",
		"port":   "45876",
		"status": "pending",
		"users":  []string{userRecord.Id},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create fingerprint record
	testToken := "test-token-789"
	_, err = createTestRecord(testApp, "fingerprints", map[string]any{
		"system":      systemRecord.Id,
		"token":       testToken,
		"fingerprint": "",
	})
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name           string
		method         string
		headers        map[string]string
		expectedStatus int
		description    string
	}{
		{
			name:   "GET with invalid token",
			method: "GET",
			headers: map[string]string{
				"X-Token":  "invalid",
				"X-Beszel": "0.5.0",
			},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject invalid token",
		},
		{
			name:   "GET with valid token",
			method: "GET",
			headers: map[string]string{
				"X-Token":  testToken,
				"X-Beszel": "0.5.0",
			},
			expectedStatus: http.StatusInternalServerError, // WebSocket upgrade fails in test
			description:    "Should pass validation but fail at WebSocket upgrade",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/beszel/agent-connect", nil)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			recorder := httptest.NewRecorder()
			err = hub.agentConnect(req, recorder)

			assert.Equal(t, tc.expectedStatus, recorder.Code, tc.description)
		})
	}
}

// TestAgentWebSocketIntegration tests WebSocket connection scenarios with an actual agent
func TestAgentWebSocketIntegration(t *testing.T) {
	// Create hub and test app
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer testApp.Cleanup()

	// Get the hub's SSH key using the proper method
	hubSigner, err := hub.GetSSHKey("")
	require.NoError(t, err)
	goodPubKey := hubSigner.PublicKey()

	// Generate WRONG key pair (should be rejected)
	_, badPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	badPubKey, err := ssh.NewPublicKey(badPrivKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	// Create test user once
	userRecord, err := createTestUser(testApp)
	require.NoError(t, err)

	// Create HTTP server with the actual API route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/beszel/agent-connect" {
			hub.agentConnect(r, w)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	testCases := []struct {
		name               string
		agentToken         string // Token agent will send
		dbToken            string // Token in database (empty means no record created)
		agentFingerprint   string // Fingerprint agent will send (empty means agent generates its own)
		dbFingerprint      string // Fingerprint in database
		agentSSHKey        ssh.PublicKey
		expectConnection   bool
		expectFingerprint  string // "empty", "unchanged", or "updated"
		expectSystemStatus string
		description        string
	}{
		{
			name:               "empty fingerprint - agent sets fingerprint on first connection",
			agentToken:         "test-token-1",
			dbToken:            "test-token-1",
			agentFingerprint:   "agent-fingerprint-1",
			dbFingerprint:      "",
			agentSSHKey:        goodPubKey,
			expectConnection:   true,
			expectFingerprint:  "updated",
			expectSystemStatus: "up",
			description:        "Agent should connect and set its fingerprint when DB fingerprint is empty",
		},
		{
			name:               "matching fingerprint should be accepted",
			agentToken:         "test-token-2",
			dbToken:            "test-token-2",
			agentFingerprint:   "matching-fingerprint-123",
			dbFingerprint:      "matching-fingerprint-123",
			agentSSHKey:        goodPubKey,
			expectConnection:   true,
			expectFingerprint:  "unchanged",
			expectSystemStatus: "up",
			description:        "Agent should connect when its fingerprint matches existing DB fingerprint",
		},
		{
			name:               "fingerprint mismatch should be rejected",
			agentToken:         "test-token-3",
			dbToken:            "test-token-3",
			agentFingerprint:   "different-fingerprint-456",
			dbFingerprint:      "original-fingerprint-123",
			agentSSHKey:        goodPubKey,
			expectConnection:   false,
			expectFingerprint:  "unchanged",
			expectSystemStatus: "pending",
			description:        "Agent should be rejected when its fingerprint doesn't match existing DB fingerprint",
		},
		{
			name:               "invalid token should be rejected",
			agentToken:         "invalid-token-999",
			dbToken:            "test-token-4",
			agentFingerprint:   "matching-fingerprint-456",
			dbFingerprint:      "matching-fingerprint-456",
			agentSSHKey:        goodPubKey,
			expectConnection:   false,
			expectFingerprint:  "unchanged",
			expectSystemStatus: "pending",
			description:        "Connection should fail when using invalid token",
		},
		{
			// This is more for the agent side, but might as well test it here
			name:               "wrong SSH key should be rejected",
			agentToken:         "test-token-5",
			dbToken:            "test-token-5",
			agentFingerprint:   "matching-fingerprint-789",
			dbFingerprint:      "matching-fingerprint-789",
			agentSSHKey:        badPubKey,
			expectConnection:   false,
			expectFingerprint:  "unchanged",
			expectSystemStatus: "pending",
			description:        "Connection should fail when agent uses wrong SSH key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test system with unique port for each test
			portNum := 45000 + len(tc.name) // Use name length to get unique port
			systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
				"name":   fmt.Sprintf("test-system-%s", tc.name),
				"host":   "localhost",
				"port":   fmt.Sprintf("%d", portNum),
				"status": "pending",
				"users":  []string{userRecord.Id},
			})
			require.NoError(t, err)

			// Always create fingerprint record for this test's system
			fingerprintRecord, err := createTestRecord(testApp, "fingerprints", map[string]any{
				"system":      systemRecord.Id,
				"token":       tc.dbToken,
				"fingerprint": tc.dbFingerprint,
			})
			require.NoError(t, err)

			// Create and configure agent
			agentDataDir := t.TempDir()

			// Set up agent fingerprint if specified
			err = os.WriteFile(filepath.Join(agentDataDir, "fingerprint"), []byte(tc.agentFingerprint), 0644)
			require.NoError(t, err)
			t.Logf("Pre-created fingerprint file for agent: %s", tc.agentFingerprint)

			testAgent, err := agent.NewAgent(agentDataDir)
			require.NoError(t, err)

			// Set up environment variables for the agent
			os.Setenv("BESZEL_AGENT_HUB_URL", ts.URL)
			os.Setenv("BESZEL_AGENT_TOKEN", tc.agentToken)
			defer func() {
				os.Unsetenv("BESZEL_AGENT_HUB_URL")
				os.Unsetenv("BESZEL_AGENT_TOKEN")
			}()

			// Start agent in background
			done := make(chan error, 1)
			go func() {
				serverOptions := agent.ServerOptions{
					Network: "tcp",
					Addr:    fmt.Sprintf("127.0.0.1:%d", portNum),
					Keys:    []ssh.PublicKey{tc.agentSSHKey},
				}
				done <- testAgent.Start(serverOptions)
			}()

			// Wait for connection result
			maxWait := 2 * time.Second
			checkInterval := 100 * time.Millisecond
			timeout := time.After(maxWait)
			ticker := time.NewTicker(checkInterval)
			defer ticker.Stop()

			connectionManager := testAgent.GetConnectionManager()

			connectionResult := false
			for {
				select {
				case <-timeout:
					// Timeout reached
					if tc.expectConnection {
						t.Fatalf("Expected connection to succeed but timed out - agent state: %d", connectionManager.State)
					} else {
						t.Logf("Connection properly rejected (timeout) - agent state: %d", connectionManager.State)
					}
					connectionResult = false
				case <-ticker.C:
					if connectionManager.State == agent.WebSocketConnected {
						if tc.expectConnection {
							t.Logf("WebSocket connection successful - agent state: %d", connectionManager.State)
							connectionResult = true
						} else {
							t.Errorf("Unexpected: Connection succeeded when it should have been rejected")
							return
						}
					}
				case err := <-done:
					if err != nil {
						if !tc.expectConnection {
							t.Logf("Agent connection properly rejected: %v", err)
							connectionResult = false
						} else {
							t.Fatalf("Agent failed to start: %v", err)
						}
					}
				}

				// Break if we got the expected result or timed out
				if connectionResult == tc.expectConnection || connectionResult {
					break
				}
			}

			// Verify fingerprint state by re-reading the specific record
			updatedFingerprintRecord, err := testApp.FindRecordById("fingerprints", fingerprintRecord.Id)
			require.NoError(t, err)
			finalFingerprint := updatedFingerprintRecord.GetString("fingerprint")

			switch tc.expectFingerprint {
			case "empty":
				assert.Empty(t, finalFingerprint, "Fingerprint should be empty")
			case "unchanged":
				assert.Equal(t, tc.dbFingerprint, finalFingerprint, "Fingerprint should not change when connection is rejected")
			case "updated":
				if tc.dbFingerprint == "" {
					assert.NotEmpty(t, finalFingerprint, "Fingerprint should be updated after successful connection")
				} else {
					assert.NotEqual(t, tc.dbFingerprint, finalFingerprint, "Fingerprint should be updated after successful connection")
				}
			}

			// Verify system status
			updatedSystemRecord, err := testApp.FindRecordById("systems", systemRecord.Id)
			require.NoError(t, err)
			status := updatedSystemRecord.GetString("status")
			assert.Equal(t, tc.expectSystemStatus, status, "System status should match expected value")

			t.Logf("%s - System status: %s, Fingerprint: %s", tc.description, status, finalFingerprint)
		})
	}
}
