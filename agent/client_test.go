//go:build testing
// +build testing

package agent

import (
	"crypto/ed25519"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel"

	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// TestNewWebSocketClient tests WebSocket client creation
func TestNewWebSocketClient(t *testing.T) {
	agent := createTestAgent(t)

	testCases := []struct {
		name        string
		hubURL      string
		token       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid configuration",
			hubURL:      "http://localhost:8080",
			token:       "test-token-123",
			expectError: false,
		},
		{
			name:        "valid https URL",
			hubURL:      "https://hub.example.com",
			token:       "secure-token",
			expectError: false,
		},
		{
			name:        "missing hub URL",
			hubURL:      "",
			token:       "test-token",
			expectError: true,
			errorMsg:    "HUB_URL environment variable not set",
		},
		{
			name:        "invalid URL",
			hubURL:      "ht\ttp://invalid",
			token:       "test-token",
			expectError: true,
			errorMsg:    "invalid hub URL",
		},
		{
			name:        "missing token",
			hubURL:      "http://localhost:8080",
			token:       "",
			expectError: true,
			errorMsg:    "must set TOKEN or TOKEN_FILE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment
			if tc.hubURL != "" {
				os.Setenv("BESZEL_AGENT_HUB_URL", tc.hubURL)
			} else {
				os.Unsetenv("BESZEL_AGENT_HUB_URL")
			}
			if tc.token != "" {
				os.Setenv("BESZEL_AGENT_TOKEN", tc.token)
			} else {
				os.Unsetenv("BESZEL_AGENT_TOKEN")
			}
			defer func() {
				os.Unsetenv("BESZEL_AGENT_HUB_URL")
				os.Unsetenv("BESZEL_AGENT_TOKEN")
			}()

			client, err := newWebSocketClient(agent)

			if tc.expectError {
				assert.Error(t, err)
				if err != nil && tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, agent, client.agent)
				assert.Equal(t, tc.token, client.token)
				assert.Equal(t, tc.hubURL, client.hubURL.String())
				assert.NotEmpty(t, client.fingerprint)
				assert.NotNil(t, client.hubRequest)
			}
		})
	}
}

// TestWebSocketClient_GetOptions tests WebSocket client options configuration
func TestWebSocketClient_GetOptions(t *testing.T) {
	agent := createTestAgent(t)

	testCases := []struct {
		name           string
		inputURL       string
		expectedScheme string
		expectedPath   string
	}{
		{
			name:           "http to ws conversion",
			inputURL:       "http://localhost:8080",
			expectedScheme: "ws",
			expectedPath:   "/api/beszel/agent-connect",
		},
		{
			name:           "https to wss conversion",
			inputURL:       "https://hub.example.com",
			expectedScheme: "wss",
			expectedPath:   "/api/beszel/agent-connect",
		},
		{
			name:           "existing path preservation",
			inputURL:       "http://localhost:8080/custom/path",
			expectedScheme: "ws",
			expectedPath:   "/custom/path/api/beszel/agent-connect",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment
			os.Setenv("BESZEL_AGENT_HUB_URL", tc.inputURL)
			os.Setenv("BESZEL_AGENT_TOKEN", "test-token")
			defer func() {
				os.Unsetenv("BESZEL_AGENT_HUB_URL")
				os.Unsetenv("BESZEL_AGENT_TOKEN")
			}()

			client, err := newWebSocketClient(agent)
			require.NoError(t, err)

			options := client.getOptions()

			// Parse the WebSocket URL
			wsURL, err := url.Parse(options.Addr)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedScheme, wsURL.Scheme)
			assert.Equal(t, tc.expectedPath, wsURL.Path)

			// Check headers
			assert.Equal(t, "test-token", options.RequestHeader.Get("X-Token"))
			assert.Equal(t, beszel.Version, options.RequestHeader.Get("X-Beszel"))
			assert.Contains(t, options.RequestHeader.Get("User-Agent"), "Mozilla/5.0")

			// Test options caching
			options2 := client.getOptions()
			assert.Same(t, options, options2, "Options should be cached")
		})
	}
}

// TestWebSocketClient_VerifySignature tests signature verification
func TestWebSocketClient_VerifySignature(t *testing.T) {
	agent := createTestAgent(t)

	// Generate test key pairs
	_, goodPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	goodPubKey, err := ssh.NewPublicKey(goodPrivKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	_, badPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	badPubKey, err := ssh.NewPublicKey(badPrivKey.Public().(ed25519.PublicKey))
	require.NoError(t, err)

	// Set up environment
	os.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	os.Setenv("BESZEL_AGENT_TOKEN", "test-token")
	defer func() {
		os.Unsetenv("BESZEL_AGENT_HUB_URL")
		os.Unsetenv("BESZEL_AGENT_TOKEN")
	}()

	client, err := newWebSocketClient(agent)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		keys        []ssh.PublicKey
		token       string
		signWith    ed25519.PrivateKey
		expectError bool
	}{
		{
			name:        "valid signature with correct key",
			keys:        []ssh.PublicKey{goodPubKey},
			token:       "test-token",
			signWith:    goodPrivKey,
			expectError: false,
		},
		{
			name:        "invalid signature with wrong key",
			keys:        []ssh.PublicKey{goodPubKey},
			token:       "test-token",
			signWith:    badPrivKey,
			expectError: true,
		},
		{
			name:        "valid signature with multiple keys",
			keys:        []ssh.PublicKey{badPubKey, goodPubKey},
			token:       "test-token",
			signWith:    goodPrivKey,
			expectError: false,
		},
		{
			name:        "no valid keys",
			keys:        []ssh.PublicKey{badPubKey},
			token:       "test-token",
			signWith:    goodPrivKey,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up agent with test keys
			agent.keys = tc.keys
			client.token = tc.token

			// Create signature
			signature := ed25519.Sign(tc.signWith, []byte(tc.token))

			err := client.verifySignature(signature)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid signature")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestWebSocketClient_HandleHubRequest tests hub request routing (basic verification logic)
func TestWebSocketClient_HandleHubRequest(t *testing.T) {
	agent := createTestAgent(t)

	// Set up environment
	os.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	os.Setenv("BESZEL_AGENT_TOKEN", "test-token")
	defer func() {
		os.Unsetenv("BESZEL_AGENT_HUB_URL")
		os.Unsetenv("BESZEL_AGENT_TOKEN")
	}()

	client, err := newWebSocketClient(agent)
	require.NoError(t, err)

	testCases := []struct {
		name        string
		action      common.WebSocketAction
		hubVerified bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "CheckFingerprint without verification",
			action:      common.CheckFingerprint,
			hubVerified: false,
			expectError: false, // CheckFingerprint is allowed without verification
		},
		{
			name:        "GetData without verification",
			action:      common.GetData,
			hubVerified: false,
			expectError: true,
			errorMsg:    "hub not verified",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client.hubVerified = tc.hubVerified

			// Create minimal request
			hubRequest := &common.HubRequest[cbor.RawMessage]{
				Action: tc.action,
				Data:   cbor.RawMessage{},
			}

			err := client.handleHubRequest(hubRequest)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				// For CheckFingerprint, we expect a decode error since we're not providing valid data,
				// but it shouldn't be the "hub not verified" error
				if err != nil && tc.errorMsg != "" {
					assert.NotContains(t, err.Error(), tc.errorMsg)
				}
			}
		})
	}
}

// TestWebSocketClient_GetUserAgent tests user agent generation
func TestGetUserAgent(t *testing.T) {
	// Run multiple times to check both variants
	userAgents := make(map[string]bool)

	for range 20 {
		ua := getUserAgent()
		userAgents[ua] = true

		// Check that it's a valid Mozilla user agent
		assert.Contains(t, ua, "Mozilla/5.0")
		assert.Contains(t, ua, "AppleWebKit/537.36")
		assert.Contains(t, ua, "Chrome/124.0.0.0")
		assert.Contains(t, ua, "Safari/537.36")

		// Should contain either Windows or Mac
		isWindows := strings.Contains(ua, "Windows NT 11.0")
		isMac := strings.Contains(ua, "Macintosh; Intel Mac OS X 14_0_0")
		assert.True(t, isWindows || isMac, "User agent should contain either Windows or Mac identifier")
	}

	// With enough iterations, we should see both variants
	// though this might occasionally fail
	if len(userAgents) == 1 {
		t.Log("Note: Only one user agent variant was generated in this test run")
	}
}

// TestWebSocketClient_Close tests connection closing
func TestWebSocketClient_Close(t *testing.T) {
	agent := createTestAgent(t)

	// Set up environment
	os.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	os.Setenv("BESZEL_AGENT_TOKEN", "test-token")
	defer func() {
		os.Unsetenv("BESZEL_AGENT_HUB_URL")
		os.Unsetenv("BESZEL_AGENT_TOKEN")
	}()

	client, err := newWebSocketClient(agent)
	require.NoError(t, err)

	// Test closing with nil connection (should not panic)
	assert.NotPanics(t, func() {
		client.Close()
	})
}

// TestWebSocketClient_ConnectRateLimit tests connection rate limiting
func TestWebSocketClient_ConnectRateLimit(t *testing.T) {
	agent := createTestAgent(t)

	// Set up environment
	os.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	os.Setenv("BESZEL_AGENT_TOKEN", "test-token")
	defer func() {
		os.Unsetenv("BESZEL_AGENT_HUB_URL")
		os.Unsetenv("BESZEL_AGENT_TOKEN")
	}()

	client, err := newWebSocketClient(agent)
	require.NoError(t, err)

	// Set recent connection attempt
	client.lastConnectAttempt = time.Now()

	// Test that connection fails quickly due to rate limiting
	// This won't actually connect but should fail fast
	err = client.Connect()
	assert.Error(t, err, "Connection should fail but not hang")
}

// TestGetToken tests the getToken function with various scenarios
func TestGetToken(t *testing.T) {
	unsetEnvVars := func() {
		os.Unsetenv("BESZEL_AGENT_TOKEN")
		os.Unsetenv("TOKEN")
		os.Unsetenv("BESZEL_AGENT_TOKEN_FILE")
		os.Unsetenv("TOKEN_FILE")
	}

	t.Run("token from TOKEN environment variable", func(t *testing.T) {
		unsetEnvVars()

		// Set TOKEN env var
		expectedToken := "test-token-from-env"
		os.Setenv("TOKEN", expectedToken)
		defer os.Unsetenv("TOKEN")

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("token from BESZEL_AGENT_TOKEN environment variable", func(t *testing.T) {
		unsetEnvVars()

		// Set BESZEL_AGENT_TOKEN env var (should take precedence)
		expectedToken := "test-token-from-beszel-env"
		os.Setenv("BESZEL_AGENT_TOKEN", expectedToken)
		defer os.Unsetenv("BESZEL_AGENT_TOKEN")

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("token from TOKEN_FILE", func(t *testing.T) {
		unsetEnvVars()

		// Create a temporary token file
		expectedToken := "test-token-from-file"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(expectedToken)
		require.NoError(t, err)
		tokenFile.Close()

		// Set TOKEN_FILE env var
		os.Setenv("TOKEN_FILE", tokenFile.Name())
		defer os.Unsetenv("TOKEN_FILE")

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("token from BESZEL_AGENT_TOKEN_FILE", func(t *testing.T) {
		unsetEnvVars()

		// Create a temporary token file
		expectedToken := "test-token-from-beszel-file"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(expectedToken)
		require.NoError(t, err)
		tokenFile.Close()

		// Set BESZEL_AGENT_TOKEN_FILE env var (should take precedence)
		os.Setenv("BESZEL_AGENT_TOKEN_FILE", tokenFile.Name())
		defer os.Unsetenv("BESZEL_AGENT_TOKEN_FILE")

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("TOKEN takes precedence over TOKEN_FILE", func(t *testing.T) {
		unsetEnvVars()

		// Create a temporary token file
		fileToken := "token-from-file"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(fileToken)
		require.NoError(t, err)
		tokenFile.Close()

		// Set both TOKEN and TOKEN_FILE
		envToken := "token-from-env"
		os.Setenv("TOKEN", envToken)
		os.Setenv("TOKEN_FILE", tokenFile.Name())
		defer func() {
			os.Unsetenv("TOKEN")
			os.Unsetenv("TOKEN_FILE")
		}()

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, envToken, token, "TOKEN should take precedence over TOKEN_FILE")
	})

	t.Run("error when neither TOKEN nor TOKEN_FILE is set", func(t *testing.T) {
		unsetEnvVars()

		token, err := getToken()
		assert.Error(t, err)
		assert.Equal(t, "", token)
		assert.Contains(t, err.Error(), "must set TOKEN or TOKEN_FILE")
	})

	t.Run("error when TOKEN_FILE points to non-existent file", func(t *testing.T) {
		unsetEnvVars()

		// Set TOKEN_FILE to a non-existent file
		os.Setenv("TOKEN_FILE", "/non/existent/file.txt")
		defer os.Unsetenv("TOKEN_FILE")

		token, err := getToken()
		assert.Error(t, err)
		assert.Equal(t, "", token)
		assert.Contains(t, err.Error(), "no such file or directory")
	})

	t.Run("handles empty token file", func(t *testing.T) {
		unsetEnvVars()

		// Create an empty token file
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())
		tokenFile.Close()

		// Set TOKEN_FILE env var
		os.Setenv("TOKEN_FILE", tokenFile.Name())
		defer os.Unsetenv("TOKEN_FILE")

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, "", token, "Empty file should return empty string")
	})

	t.Run("strips whitespace from TOKEN_FILE", func(t *testing.T) {
		unsetEnvVars()

		tokenWithWhitespace := "  test-token-with-whitespace  \n\t"
		expectedToken := "test-token-with-whitespace"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(tokenWithWhitespace)
		require.NoError(t, err)
		tokenFile.Close()

		os.Setenv("TOKEN_FILE", tokenFile.Name())
		defer os.Unsetenv("TOKEN_FILE")

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token, "Whitespace should be stripped from token file content")
	})
}
