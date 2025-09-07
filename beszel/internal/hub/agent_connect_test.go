//go:build testing
// +build testing

package hub

import (
	"crypto/ed25519"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/agent"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/hub/ws"

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
				"X-Token":  []string{strings.Repeat("a", 65)},
				"X-Beszel": []string{"0.5.0"},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			acr := &agentConnectRequest{hub: hub}
			token, agentVersion, err := acr.validateAgentHeaders(tc.headers)

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

// TestGetAllFingerprintRecordsByToken tests the getAllFingerprintRecordsByToken function
func TestGetAllFingerprintRecordsByToken(t *testing.T) {
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
	for i := range 3 {
		systemRecord, _ := createTestRecord(testApp, "systems", map[string]any{
			"name":   fmt.Sprintf("test-system-%d", i),
			"host":   "localhost",
			"port":   "45876",
			"status": "pending",
			"users":  []string{userRecord.Id},
		})
		createTestRecord(testApp, "fingerprints", map[string]any{
			"system":      systemRecord.Id,
			"token":       "duplicate-token",
			"fingerprint": fmt.Sprintf("test-fingerprint-%d", i),
		})
	}
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name       string
		token      string
		expectedId string
		expectLen  int
	}{
		{
			name:       "valid token",
			token:      "test-token-123",
			expectLen:  1,
			expectedId: fingerprintRecord.Id,
		},
		{
			name:      "invalid token",
			token:     "invalid-token",
			expectLen: 0,
		},
		{
			name:      "empty token",
			token:     "",
			expectLen: 0,
		},
		{
			name:      "duplicate token",
			token:     "duplicate-token",
			expectLen: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			records := getFingerprintRecordsByToken(tc.token, hub)

			require.Len(t, records, tc.expectLen)
			if tc.expectedId != "" {
				assert.Equal(t, tc.expectedId, records[0].Id)
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
				hub:    hub,
				userId: userRecord.Id,
				req: &http.Request{
					RemoteAddr: "192.168.0.1",
				},
			},
			fingerprint: common.FingerprintResponse{
				Hostname: "test-server",
				Port:     "8080",
			},
			expectError:   false,
			expectedName:  "test-server",
			expectedHost:  "192.168.0.1", // This will be the parsed IP from the mock request
			expectedPort:  "8080",
			expectedUsers: []string{userRecord.Id},
		},
		{
			name: "system creation with default port",
			agentConnReq: agentConnectRequest{
				hub:    hub,
				userId: userRecord.Id,
				req: &http.Request{
					RemoteAddr: "192.168.0.1",
				},
			},
			fingerprint: common.FingerprintResponse{
				Hostname: "default-port-server",
				Port:     "", // Empty port should default to 45876
			},
			expectError:   false,
			expectedName:  "default-port-server",
			expectedHost:  "192.168.0.1", // This will be the parsed IP from the mock request
			expectedPort:  "45876",
			expectedUsers: []string{userRecord.Id},
		},
		{
			name: "system creation with empty hostname",
			agentConnReq: agentConnectRequest{
				hub:    hub,
				userId: userRecord.Id,
				req: &http.Request{
					RemoteAddr: "192.168.0.1",
				},
			},
			fingerprint: common.FingerprintResponse{
				Hostname: "",
				Port:     "9090",
			},
			expectError:   false,
			expectedName:  "192.168.0.1", // Should fall back to host IP when hostname is empty
			expectedHost:  "192.168.0.1", // This will be the parsed IP from the mock request
			expectedPort:  "9090",
			expectedUsers: []string{userRecord.Id},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recordId, err := tc.agentConnReq.createSystem(tc.fingerprint)

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

	universalTokenMap.GetMap().Set(universalToken, userRecord.Id, time.Hour)

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
			acr := &agentConnectRequest{}

			acr.userId, acr.isUniversalToken = universalTokenMap.GetMap().GetOk(tc.token)

			if tc.expectError {
				assert.False(t, acr.isUniversalToken)
				assert.Empty(t, acr.userId)
			} else {
				assert.Equal(t, tc.expectUniversalAuth, acr.isUniversalToken)
				if tc.expectUniversalAuth {
					assert.Equal(t, userRecord.Id, acr.userId)
				}
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
		errorMessage   string
	}{
		{
			name: "missing token header",
			headers: map[string]string{
				"X-Beszel": "0.5.0",
			},
			expectedStatus: http.StatusBadRequest,
			description:    "Should fail due to missing token",
			errorMessage:   "",
		},
		{
			name: "missing agent version header",
			headers: map[string]string{
				"X-Token": testToken,
			},
			expectedStatus: http.StatusBadRequest,
			description:    "Should fail due to missing agent version",
			errorMessage:   "",
		},
		{
			name: "invalid token",
			headers: map[string]string{
				"X-Token":  "invalid-token",
				"X-Beszel": "0.5.0",
			},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should fail due to invalid token",
			errorMessage:   "Invalid token",
		},
		{
			name: "invalid agent version",
			headers: map[string]string{
				"X-Token":  testToken,
				"X-Beszel": "0.5.0.0.0",
			},
			expectedStatus: http.StatusUnauthorized,
			description:    "Should fail due to invalid agent version",
			errorMessage:   "Invalid agent version",
		},
		{
			name: "valid headers but websocket upgrade will fail in test",
			headers: map[string]string{
				"X-Token":  testToken,
				"X-Beszel": "0.5.0",
			},
			expectedStatus: http.StatusInternalServerError,
			description:    "Should pass validation but fail at WebSocket upgrade due to test limitations",
			errorMessage:   "WebSocket upgrade failed",
		},
		{
			name:           "Token too long",
			headers:        map[string]string{"X-Token": strings.Repeat("a", 65), "X-Beszel": "0.5.0"},
			expectedStatus: http.StatusBadRequest,
			description:    "Should reject token exceeding 64 characters",
			errorMessage:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/beszel/agent-connect", nil)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			recorder := httptest.NewRecorder()
			acr := &agentConnectRequest{
				hub: hub,
				req: req,
				res: recorder,
			}
			err = acr.agentConnect()

			assert.Equal(t, tc.expectedStatus, recorder.Code, tc.description)
			assert.Equal(t, tc.errorMessage, recorder.Body.String(), tc.description)
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
			acr := &agentConnectRequest{}
			acr.sendResponseError(recorder, tc.statusCode, tc.message)

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
			acr := &agentConnectRequest{
				hub: hub,
				req: req,
				res: recorder,
			}
			err = acr.agentConnect()

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

	// Get the hub's SSH key
	hubSigner, err := hub.GetSSHKey("")
	require.NoError(t, err)
	goodPubKey := hubSigner.PublicKey()

	// Generate bad key pair (should be rejected)
	_, badPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	badPubKey, err := ssh.NewPublicKey(badPrivKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	// Create test user
	userRecord, err := createTestUser(testApp)
	require.NoError(t, err)

	// Create HTTP server with the actual API route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/beszel/agent-connect" {
			acr := &agentConnectRequest{
				hub: hub,
				req: r,
				res: w,
			}
			acr.agentConnect()
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
			time.Sleep(20 * time.Millisecond)
			checkInterval := 20 * time.Millisecond
			timeout := time.After(maxWait)
			ticker := time.Tick(checkInterval)

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
				case <-ticker:
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

// TestMultipleSystemsWithSameUniversalToken tests that multiple systems can share the same universal token
func TestMultipleSystemsWithSameUniversalToken(t *testing.T) {
	// Create hub and test app
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer testApp.Cleanup()

	// Get the hub's SSH key
	hubSigner, err := hub.GetSSHKey("")
	require.NoError(t, err)
	goodPubKey := hubSigner.PublicKey()

	// Create test user
	userRecord, err := createTestUser(testApp)
	require.NoError(t, err)

	// Set up universal token in the token map
	universalToken := "shared-universal-token-123"
	universalTokenMap.GetMap().Set(universalToken, userRecord.Id, time.Hour)

	// Create HTTP server with the actual API route
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/beszel/agent-connect" {
			acr := &agentConnectRequest{
				hub: hub,
				req: r,
				res: w,
			}
			acr.agentConnect()
		} else {
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Test scenarios for universal tokens
	testCases := []struct {
		name               string
		agentFingerprint   string
		expectConnection   bool
		expectSystemStatus string
		expectNewSystem    bool // Whether we expect a new system to be created
		description        string
	}{
		{
			name:               "first system with universal token",
			agentFingerprint:   "system-1-fingerprint",
			expectConnection:   true,
			expectSystemStatus: "up",
			expectNewSystem:    true,
			description:        "First system should create a new system",
		},
		{
			name:               "same system reconnecting with same fingerprint",
			agentFingerprint:   "system-1-fingerprint", // Same fingerprint as first
			expectConnection:   true,
			expectSystemStatus: "up",
			expectNewSystem:    false, // Should reuse existing system
			description:        "Same system should reuse existing system record",
		},
		{
			name:               "different system with same universal token",
			agentFingerprint:   "system-2-fingerprint", // Different fingerprint
			expectConnection:   true,
			expectSystemStatus: "up",
			expectNewSystem:    true, // Should create new system
			description:        "Different system should create a new system record",
		},
	}

	var systemCount int
	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create unique port for each test
			portNum := 46000 + i

			// Create and configure agent
			agentDataDir := t.TempDir()

			// Set up agent fingerprint
			err = os.WriteFile(filepath.Join(agentDataDir, "fingerprint"), []byte(tc.agentFingerprint), 0644)
			require.NoError(t, err)

			testAgent, err := agent.NewAgent(agentDataDir)
			require.NoError(t, err)

			// Set up environment variables for the agent
			os.Setenv("BESZEL_AGENT_HUB_URL", ts.URL)
			os.Setenv("BESZEL_AGENT_TOKEN", universalToken)
			defer func() {
				os.Unsetenv("BESZEL_AGENT_HUB_URL")
				os.Unsetenv("BESZEL_AGENT_TOKEN")
			}()

			// Count systems before connection
			systemsBefore, err := testApp.FindRecordsByFilter("systems", "users ~ {:userId}", "", -1, 0, map[string]any{"userId": userRecord.Id})
			require.NoError(t, err)
			systemsBeforeCount := len(systemsBefore)

			// Start agent in background
			done := make(chan error, 1)
			go func() {
				serverOptions := agent.ServerOptions{
					Network: "tcp",
					Addr:    fmt.Sprintf("127.0.0.1:%d", portNum),
					Keys:    []ssh.PublicKey{goodPubKey},
				}
				done <- testAgent.Start(serverOptions)
			}()

			// Wait for connection result
			maxWait := 2 * time.Second
			time.Sleep(20 * time.Millisecond)
			checkInterval := 20 * time.Millisecond
			timeout := time.After(maxWait)
			ticker := time.Tick(checkInterval)

			connectionManager := testAgent.GetConnectionManager()
			connectionResult := false

			for {
				select {
				case <-timeout:
					if tc.expectConnection {
						t.Fatalf("Expected connection to succeed but timed out - agent state: %d", connectionManager.State)
					} else {
						t.Logf("Connection properly rejected (timeout) - agent state: %d", connectionManager.State)
					}
					connectionResult = false
				case <-ticker:
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

				if connectionResult == tc.expectConnection || connectionResult {
					break
				}
			}

			// Verify system creation/reuse behavior
			if tc.expectConnection {
				// Count systems after connection
				systemsAfter, err := testApp.FindRecordsByFilter("systems", "users ~ {:userId}", "", -1, 0, map[string]any{"userId": userRecord.Id})
				require.NoError(t, err)
				systemsAfterCount := len(systemsAfter)

				if tc.expectNewSystem {
					// Should have created a new system
					systemCount++
					assert.Equal(t, systemsBeforeCount+1, systemsAfterCount, "Should have created a new system")
					assert.Equal(t, systemCount, systemsAfterCount, "Total system count should match expected")
				} else {
					// Should have reused existing system
					assert.Equal(t, systemsBeforeCount, systemsAfterCount, "Should not have created a new system")
					assert.Equal(t, systemCount, systemsAfterCount, "Total system count should remain the same")
				}

				// Verify that a fingerprint record exists for this fingerprint
				fingerprints, err := testApp.FindRecordsByFilter("fingerprints", "token = {:token} && fingerprint = {:fingerprint}", "", -1, 0, map[string]any{
					"token":       universalToken,
					"fingerprint": tc.agentFingerprint,
				})
				require.NoError(t, err)
				require.Len(t, fingerprints, 1, "Should have exactly one fingerprint record for this token+fingerprint combination")

				fingerprint := fingerprints[0]
				assert.Equal(t, universalToken, fingerprint.GetString("token"), "Fingerprint should have the universal token")
				assert.Equal(t, tc.agentFingerprint, fingerprint.GetString("fingerprint"), "Fingerprint should match agent's fingerprint")

				// Verify system status
				systemId := fingerprint.GetString("system")
				system, err := testApp.FindRecordById("systems", systemId)
				require.NoError(t, err)
				status := system.GetString("status")
				assert.Equal(t, tc.expectSystemStatus, status, "System status should match expected value")

				t.Logf("%s - System ID: %s, Status: %s, New System: %v", tc.description, systemId, status, tc.expectNewSystem)
			}
		})
	}
}

// TestFindOrCreateSystemForToken tests the findOrCreateSystemForToken function
func TestFindOrCreateSystemForToken(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer testApp.Cleanup()

	// Create test user
	userRecord, err := createTestUser(testApp)
	require.NoError(t, err)

	type testCase struct {
		name                string
		setup               func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord)
		agentFingerprint    common.FingerprintResponse
		expectError         bool
		expectNewSystem     bool
		expectedFingerprint string
		description         string
	}

	testCases := []testCase{
		{
			name: "universal token - existing fingerprint match",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				// Create test system
				systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
					"name":   "existing-system",
					"host":   "192.168.1.100",
					"port":   "45876",
					"status": "pending",
					"users":  []string{userRecord.Id},
				})
				require.NoError(t, err)

				// Create fingerprint record
				fpRecord, err := createTestRecord(testApp, "fingerprints", map[string]any{
					"system":      systemRecord.Id,
					"token":       "universal-token-123",
					"fingerprint": "existing-fingerprint",
				})
				require.NoError(t, err)

				acr := agentConnectRequest{
					hub:              hub,
					token:            "universal-token-123",
					isUniversalToken: true,
					userId:           userRecord.Id,
					req: &http.Request{
						RemoteAddr: "192.168.1.100",
					},
				}

				fpRecords := []ws.FingerprintRecord{
					{
						Id:          fpRecord.Id,
						SystemId:    systemRecord.Id,
						Fingerprint: "existing-fingerprint",
						Token:       "universal-token-123",
					},
				}

				return acr, fpRecords
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "existing-fingerprint",
				Hostname:    "test-host",
				Port:        "8080",
			},
			expectError:         false,
			expectNewSystem:     false,
			expectedFingerprint: "existing-fingerprint",
			description:         "Should reuse existing system with matching fingerprint",
		},
		{
			name: "universal token - new fingerprint",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				// Create test system
				systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
					"name":   "existing-system-2",
					"host":   "192.168.1.101",
					"port":   "45876",
					"status": "pending",
					"users":  []string{userRecord.Id},
				})
				require.NoError(t, err)

				// Create fingerprint record
				fpRecord, err := createTestRecord(testApp, "fingerprints", map[string]any{
					"system":      systemRecord.Id,
					"token":       "universal-token-123",
					"fingerprint": "existing-fingerprint",
				})
				require.NoError(t, err)

				acr := agentConnectRequest{
					hub:              hub,
					token:            "universal-token-123",
					isUniversalToken: true,
					userId:           userRecord.Id,
					req: &http.Request{
						RemoteAddr: "192.168.1.200",
					},
				}

				fpRecords := []ws.FingerprintRecord{
					{
						Id:          fpRecord.Id,
						SystemId:    systemRecord.Id,
						Fingerprint: "existing-fingerprint",
						Token:       "universal-token-123",
					},
				}

				return acr, fpRecords
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "new-fingerprint",
				Hostname:    "new-host",
				Port:        "9090",
			},
			expectError:         false,
			expectNewSystem:     true,
			expectedFingerprint: "new-fingerprint",
			description:         "Should create new system with different fingerprint",
		},
		{
			name: "universal token - no existing records",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				acr := agentConnectRequest{
					hub:              hub,
					token:            "universal-token-456",
					isUniversalToken: true,
					userId:           userRecord.Id,
					req: &http.Request{
						RemoteAddr: "192.168.1.300",
					},
				}

				fpRecords := []ws.FingerprintRecord{}

				return acr, fpRecords
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "first-fingerprint",
				Hostname:    "first-host",
				Port:        "7070",
			},
			expectError:         false,
			expectNewSystem:     true,
			expectedFingerprint: "first-fingerprint",
			description:         "Should create new system when no existing records",
		},
		{
			name: "regular token - empty fingerprint",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				// Create test system
				systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
					"name":   "regular-system",
					"host":   "192.168.1.200",
					"port":   "45876",
					"status": "pending",
					"users":  []string{userRecord.Id},
				})
				require.NoError(t, err)

				// Create fingerprint record with empty fingerprint
				fpRecord, err := createTestRecord(testApp, "fingerprints", map[string]any{
					"system":      systemRecord.Id,
					"token":       "regular-token-123",
					"fingerprint": "",
				})
				require.NoError(t, err)

				acr := agentConnectRequest{
					hub:              hub,
					token:            "regular-token-123",
					isUniversalToken: false,
				}

				fpRecords := []ws.FingerprintRecord{
					{
						Id:          fpRecord.Id,
						SystemId:    systemRecord.Id,
						Fingerprint: "",
						Token:       "regular-token-123",
					},
				}

				return acr, fpRecords
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "agent-fingerprint",
				Hostname:    "agent-host",
				Port:        "6060",
			},
			expectError:         false,
			expectNewSystem:     false,
			expectedFingerprint: "agent-fingerprint",
			description:         "Should update empty fingerprint for regular token",
		},
		{
			name: "regular token - fingerprint mismatch",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				// Create test system
				systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
					"name":   "regular-system-2",
					"host":   "192.168.1.250",
					"port":   "45876",
					"status": "pending",
					"users":  []string{userRecord.Id},
				})
				require.NoError(t, err)

				// Create fingerprint record with different fingerprint
				fpRecord, err := createTestRecord(testApp, "fingerprints", map[string]any{
					"system":      systemRecord.Id,
					"token":       "regular-token-456",
					"fingerprint": "different-fingerprint",
				})
				require.NoError(t, err)

				acr := agentConnectRequest{
					hub:              hub,
					token:            "regular-token-456",
					isUniversalToken: false,
				}

				fpRecords := []ws.FingerprintRecord{
					{
						Id:          fpRecord.Id,
						SystemId:    systemRecord.Id,
						Fingerprint: "different-fingerprint",
						Token:       "regular-token-456",
					},
				}

				return acr, fpRecords
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "agent-fingerprint",
				Hostname:    "agent-host",
				Port:        "5050",
			},
			expectError: true,
			description: "Should reject fingerprint mismatch for regular token",
		},
		{
			name: "universal token - missing user ID",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				acr := agentConnectRequest{
					hub:              hub,
					token:            "universal-token-789",
					isUniversalToken: true,
					userId:           "", // Missing user ID
					req: &http.Request{
						RemoteAddr: "192.168.1.400",
					},
				}

				fpRecords := []ws.FingerprintRecord{}

				return acr, fpRecords
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "some-fingerprint",
				Hostname:    "some-host",
				Port:        "4040",
			},
			expectError: true,
			description: "Should reject universal token without user ID",
		},
		{
			name: "expired universal token - matching fingerprint",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				// Create test systems
				systemRecord1, err := createTestRecord(testApp, "systems", map[string]any{
					"name":   "expired-system-1",
					"host":   "192.168.1.500",
					"port":   "45876",
					"status": "pending",
					"users":  []string{userRecord.Id},
				})
				require.NoError(t, err)

				systemRecord2, err := createTestRecord(testApp, "systems", map[string]any{
					"name":   "expired-system-2",
					"host":   "192.168.1.501",
					"port":   "45876",
					"status": "pending",
					"users":  []string{userRecord.Id},
				})
				require.NoError(t, err)

				// Create fingerprint records
				fpRecord1, err := createTestRecord(testApp, "fingerprints", map[string]any{
					"system":      systemRecord1.Id,
					"token":       "expired-universal-token-123",
					"fingerprint": "expired-fingerprint-1",
				})
				require.NoError(t, err)

				fpRecord2, err := createTestRecord(testApp, "fingerprints", map[string]any{
					"system":      systemRecord2.Id,
					"token":       "expired-universal-token-123",
					"fingerprint": "expired-fingerprint-2",
				})
				require.NoError(t, err)

				acr := agentConnectRequest{
					hub:              hub,
					token:            "expired-universal-token-123",
					isUniversalToken: false, // Token is no longer active
					userId:           "",    // No user ID since token is expired
				}

				fpRecords := []ws.FingerprintRecord{
					{
						Id:          fpRecord1.Id,
						SystemId:    systemRecord1.Id,
						Fingerprint: "expired-fingerprint-1",
						Token:       "expired-universal-token-123",
					},
					{
						Id:          fpRecord2.Id,
						SystemId:    systemRecord2.Id,
						Fingerprint: "expired-fingerprint-2",
						Token:       "expired-universal-token-123",
					},
				}

				return acr, fpRecords
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "expired-fingerprint-1", // Matches first record
				Hostname:    "expired-host",
				Port:        "3030",
			},
			expectError:         false,
			expectNewSystem:     false,
			expectedFingerprint: "expired-fingerprint-1",
			description:         "Should allow connection with expired universal token if fingerprint matches",
		},
		{
			name: "expired universal token - no matching fingerprint",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				// Create test system
				systemRecord, err := createTestRecord(testApp, "systems", map[string]any{
					"name":   "expired-system-3",
					"host":   "192.168.1.600",
					"port":   "45876",
					"status": "pending",
					"users":  []string{userRecord.Id},
				})
				require.NoError(t, err)

				// Create fingerprint record
				fpRecord, err := createTestRecord(testApp, "fingerprints", map[string]any{
					"system":      systemRecord.Id,
					"token":       "expired-universal-token-456",
					"fingerprint": "expired-fingerprint-3",
				})
				require.NoError(t, err)

				acr := agentConnectRequest{
					hub:              hub,
					token:            "expired-universal-token-456",
					isUniversalToken: false, // Token is no longer active
					userId:           "",    // No user ID since token is expired
					req: &http.Request{
						RemoteAddr: "192.168.1.600",
					},
				}

				fpRecords := []ws.FingerprintRecord{
					{
						Id:          fpRecord.Id,
						SystemId:    systemRecord.Id,
						Fingerprint: "expired-fingerprint-3",
						Token:       "expired-universal-token-456",
					},
				}

				return acr, fpRecords
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "different-fingerprint", // Doesn't match any existing record
				Hostname:    "different-host",
				Port:        "2020",
			},
			expectError: true,
			description: "Should reject connection with expired universal token if no fingerprint matches",
		},
		{
			name: "regular token - no existing records",
			setup: func(t *testing.T, hub *Hub, testApp *pbtests.TestApp, userRecord *core.Record) (agentConnectRequest, []ws.FingerprintRecord) {
				acr := agentConnectRequest{
					hub:              hub,
					token:            "regular-token-no-record",
					isUniversalToken: false,
				}
				return acr, []ws.FingerprintRecord{}
			},
			agentFingerprint: common.FingerprintResponse{
				Fingerprint: "some-fingerprint",
			},
			expectError: true,
			description: "Should reject regular token with no fingerprint record",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			acr, fpRecords := tc.setup(t, hub, testApp, userRecord)
			result, err := acr.findOrCreateSystemForToken(fpRecords, tc.agentFingerprint)

			if tc.expectError {
				assert.Error(t, err, tc.description)
				return
			}

			require.NoError(t, err, tc.description)

			// Verify expected fingerprint
			if tc.expectedFingerprint != "" {
				assert.Equal(t, tc.expectedFingerprint, result.Fingerprint, "Fingerprint should match expected")
			}

			// For new systems, verify they were actually created
			if tc.expectNewSystem {
				assert.NotEmpty(t, result.SystemId, "New system should have a system ID")

				// Verify system was created in database
				system, err := testApp.FindRecordById("systems", result.SystemId)
				require.NoError(t, err, "New system should exist in database")

				// Verify system properties
				assert.Equal(t, tc.agentFingerprint.Hostname, system.GetString("name"), "System name should match hostname")
				assert.Equal(t, getRealIP(acr.req), system.GetString("host"), "System host should match remote address")
				assert.Equal(t, tc.agentFingerprint.Port, system.GetString("port"), "System port should match agent port")
				assert.Equal(t, []string{acr.userId}, system.Get("users"), "System users should match")
			}

			t.Logf("%s - Result: SystemId=%s, Fingerprint=%s", tc.description, result.SystemId, result.Fingerprint)
		})
	}
}

// TestGetRealIP tests the getRealIP function
func TestGetRealIP(t *testing.T) {
	testCases := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expectedIP string
	}{
		{
			name:       "CF-Connecting-IP header",
			headers:    map[string]string{"CF-Connecting-IP": "192.168.1.1"},
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For header with single IP",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.2"},
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.168.1.2",
		},
		{
			name:       "X-Forwarded-For header with multiple IPs",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.3, 10.0.0.1, 172.16.0.1"},
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.168.1.3",
		},
		{
			name:       "X-Forwarded-For header with spaces",
			headers:    map[string]string{"X-Forwarded-For": "  192.168.1.4  "},
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.168.1.4",
		},
		{
			name:       "No headers, fallback to RemoteAddr with port",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.5:54321",
			expectedIP: "192.168.1.5",
		},
		{
			name:       "No headers, fallback to RemoteAddr without port",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.6",
			expectedIP: "192.168.1.6",
		},
		{
			name:       "Both headers present, CF takes precedence",
			headers:    map[string]string{"CF-Connecting-IP": "192.168.1.1", "X-Forwarded-For": "192.168.1.2"},
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For present, takes precedence over RemoteAddr",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.2"},
			remoteAddr: "192.168.1.5:54321",
			expectedIP: "192.168.1.2",
		},
		{
			name:       "Empty X-Forwarded-For, fallback to RemoteAddr",
			headers:    map[string]string{"X-Forwarded-For": ""},
			remoteAddr: "192.168.1.7:12345",
			expectedIP: "192.168.1.7",
		},
		{
			name:       "Empty CF-Connecting-IP, fallback to X-Forwarded-For",
			headers:    map[string]string{"CF-Connecting-IP": "", "X-Forwarded-For": "192.168.1.8"},
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.168.1.8",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}
			req.RemoteAddr = tc.remoteAddr

			ip := getRealIP(req)
			assert.Equal(t, tc.expectedIP, ip)
		})
	}
}
