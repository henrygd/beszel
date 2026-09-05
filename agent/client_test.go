//go:build testing

package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/henrygd/beszel"

	"github.com/henrygd/beszel/internal/common"

	"github.com/fxamacker/cbor/v2"
	"github.com/lxzan/gws"
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
			name:        "malformed URL",
			hubURL:      "ht\ttp://invalid",
			token:       "test-token",
			expectError: true,
			errorMsg:    "invalid HUB_URL",
		},
		{
			name:        "URL without host",
			hubURL:      "http:/api",
			token:       "test-token",
			expectError: true,
			errorMsg:    "invalid HUB_URL",
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
				t.Setenv("BESZEL_AGENT_HUB_URL", tc.hubURL)
			}
			if tc.token != "" {
				t.Setenv("BESZEL_AGENT_TOKEN", tc.token)
			}

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
			t.Setenv("BESZEL_AGENT_HUB_URL", tc.inputURL)
			t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

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

func TestWebSocketClient_TLSVerification(t *testing.T) {
	agent := createTestAgent(t)
	serverCert, serverCertPEM := newSelfSignedServerCertificate(t)
	upgrader := gws.NewUpgrader(&gws.BuiltinEventHandler{}, nil)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r)
		if err == nil {
			go conn.ReadLoop()
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}}
	server.StartTLS()
	t.Cleanup(server.Close)

	caCertFile := filepath.Join(t.TempDir(), "hub-ca.crt")
	require.NoError(t, os.WriteFile(caCertFile, serverCertPEM, 0600))

	newClient := func(t *testing.T, caCertFile string) *WebSocketClient {
		t.Helper()
		t.Setenv("BESZEL_AGENT_HUB_URL", server.URL)
		t.Setenv("BESZEL_AGENT_TOKEN", "test-token")
		t.Setenv("BESZEL_AGENT_CA_CERT_FILE", caCertFile)
		client, err := newWebSocketClient(agent)
		require.NoError(t, err)
		return client
	}

	t.Run("system roots are used by default", func(t *testing.T) {
		client := newClient(t, "")
		assert.Nil(t, client.getOptions().TlsConfig)
		_, _, err := gws.NewClient(&gws.BuiltinEventHandler{}, client.getOptions())
		require.Error(t, err)
	})

	t.Run("custom CA trusts self-signed certificate", func(t *testing.T) {
		systemRoots, err := x509.SystemCertPool()
		require.NoError(t, err)
		client := newClient(t, caCertFile)
		assert.Greater(t, len(client.getOptions().TlsConfig.RootCAs.Subjects()), len(systemRoots.Subjects()))
		conn, _, err := gws.NewClient(&gws.BuiltinEventHandler{}, client.getOptions())
		require.NoError(t, err)
		require.NoError(t, conn.NetConn().Close())
	})

	t.Run("custom CA does not bypass hostname verification", func(t *testing.T) {
		client := newClient(t, caCertFile)
		client.getOptions().TlsConfig.ServerName = "wrong.example.com"
		_, _, err := gws.NewClient(&gws.BuiltinEventHandler{}, client.getOptions())
		require.Error(t, err)
	})
}

func TestWebSocketClient_NonTLSConnection(t *testing.T) {
	agent := createTestAgent(t)
	upgrader := gws.NewUpgrader(&gws.BuiltinEventHandler{}, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r)
		if err == nil {
			go conn.ReadLoop()
		}
	}))
	t.Cleanup(server.Close)

	t.Setenv("BESZEL_AGENT_HUB_URL", server.URL)
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")
	t.Setenv("BESZEL_AGENT_CA_CERT_FILE", "")
	client, err := newWebSocketClient(agent)
	require.NoError(t, err)
	assert.Nil(t, client.getOptions().TlsConfig)

	conn, _, err := gws.NewClient(&gws.BuiltinEventHandler{}, client.getOptions())
	require.NoError(t, err)
	require.NoError(t, conn.NetConn().Close())
}

func TestGetTLSConfigErrors(t *testing.T) {
	tempDir := t.TempDir()
	testCases := []struct {
		name       string
		path       string
		contents   []byte
		errorMatch string
	}{
		{
			name:       "missing file",
			path:       filepath.Join(tempDir, "missing.pem"),
			errorMatch: "read CA_CERT_FILE",
		},
		{
			name:       "unreadable path",
			path:       tempDir,
			errorMatch: "read CA_CERT_FILE",
		},
		{
			name:       "empty file",
			path:       filepath.Join(tempDir, "empty.pem"),
			contents:   []byte{},
			errorMatch: "does not contain any valid PEM certificates",
		},
		{
			name:       "malformed file",
			path:       filepath.Join(tempDir, "malformed.pem"),
			contents:   []byte("not a PEM certificate"),
			errorMatch: "does not contain any valid PEM certificates",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.contents != nil {
				require.NoError(t, os.WriteFile(tc.path, tc.contents, 0600))
			}
			t.Setenv("BESZEL_AGENT_CA_CERT_FILE", tc.path)

			tlsConfig, err := getTLSConfig()
			require.Error(t, err)
			assert.Nil(t, tlsConfig)
			assert.Contains(t, err.Error(), tc.errorMatch)
			assert.Contains(t, err.Error(), tc.path)
		})
	}
}

func newSelfSignedServerCertificate(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return certificate, certPEM
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
	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

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
	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

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

			err := client.handleHubRequest(hubRequest, nil)

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

	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

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

	t.Setenv("BESZEL_AGENT_HUB_URL", "http://localhost:8080")
	t.Setenv("BESZEL_AGENT_TOKEN", "test-token")

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
	t.Run("token from TOKEN environment variable", func(t *testing.T) {
		// Set TOKEN env var
		expectedToken := "test-token-from-env"
		t.Setenv("TOKEN", expectedToken)

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("token from BESZEL_AGENT_TOKEN environment variable", func(t *testing.T) {
		// Set BESZEL_AGENT_TOKEN env var (should take precedence)
		expectedToken := "test-token-from-beszel-env"
		t.Setenv("BESZEL_AGENT_TOKEN", expectedToken)

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("token from TOKEN_FILE", func(t *testing.T) {
		// Create a temporary token file
		expectedToken := "test-token-from-file"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(expectedToken)
		require.NoError(t, err)
		tokenFile.Close()

		// Set TOKEN_FILE env var
		t.Setenv("TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("TOKEN_FILE with surrounding blank lines and comments", func(t *testing.T) {
		expectedToken := "test-token-with-noise"
		tokenFile := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("# hub token\n\n"+expectedToken+"\n\n"), 0o600))

		t.Setenv("TOKEN_FILE", tokenFile)

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("TOKEN_FILE with multiple tokens is rejected", func(t *testing.T) {
		tokenFile := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("11111111-1111-1111-1111-111111111111\n22222222-2222-2222-2222-222222222222\n"), 0o600))

		t.Setenv("TOKEN_FILE", tokenFile)

		token, err := getToken()
		require.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "must contain a single token")
	})

	t.Run("TOKEN_FILE holding only comments behaves like an empty file", func(t *testing.T) {
		tokenFile := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("\n# only a comment\n"), 0o600))

		t.Setenv("TOKEN_FILE", tokenFile)

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, "", token)
	})

	t.Run("token from BESZEL_AGENT_TOKEN_FILE", func(t *testing.T) {
		// Create a temporary token file
		expectedToken := "test-token-from-beszel-file"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(expectedToken)
		require.NoError(t, err)
		tokenFile.Close()

		// Set BESZEL_AGENT_TOKEN_FILE env var (should take precedence)
		t.Setenv("BESZEL_AGENT_TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token)
	})

	t.Run("TOKEN takes precedence over TOKEN_FILE", func(t *testing.T) {
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
		t.Setenv("TOKEN", envToken)
		t.Setenv("TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, envToken, token, "TOKEN should take precedence over TOKEN_FILE")
	})

	t.Run("error when neither TOKEN nor TOKEN_FILE is set", func(t *testing.T) {
		t.Setenv("BESZEL_AGENT_TOKEN", "")
		t.Setenv("TOKEN", "")
		t.Setenv("BESZEL_AGENT_TOKEN_FILE", "")
		t.Setenv("TOKEN_FILE", "")

		token, err := getToken()
		assert.Error(t, err)
		assert.Equal(t, "", token)
		assert.Contains(t, err.Error(), "must set TOKEN or TOKEN_FILE")
	})

	t.Run("error when TOKEN_FILE points to non-existent file", func(t *testing.T) {
		// Set TOKEN_FILE to a non-existent file
		t.Setenv("TOKEN_FILE", "/non/existent/file.txt")

		token, err := getToken()
		assert.Error(t, err)
		assert.Equal(t, "", token)
		assert.Contains(t, err.Error(), "no such file or directory")
	})

	t.Run("handles empty token file", func(t *testing.T) {
		// Create an empty token file
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())
		tokenFile.Close()

		// Set TOKEN_FILE env var
		t.Setenv("TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, "", token, "Empty file should return empty string")
	})

	t.Run("strips whitespace from TOKEN_FILE", func(t *testing.T) {
		tokenWithWhitespace := "  test-token-with-whitespace  \n\t"
		expectedToken := "test-token-with-whitespace"
		tokenFile, err := os.CreateTemp("", "token-test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tokenFile.Name())

		_, err = tokenFile.WriteString(tokenWithWhitespace)
		require.NoError(t, err)
		tokenFile.Close()

		t.Setenv("TOKEN_FILE", tokenFile.Name())

		token, err := getToken()
		assert.NoError(t, err)
		assert.Equal(t, expectedToken, token, "Whitespace should be stripped from token file content")
	})
}
