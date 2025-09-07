package agent

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/henrygd/beszel/src/entities/container"
	"github.com/henrygd/beszel/src/entities/system"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

func TestStartServer(t *testing.T) {
	// Generate a test key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(privKey)
	require.NoError(t, err)
	sshPubKey, err := gossh.NewPublicKey(pubKey)
	require.NoError(t, err)

	// Generate a different key pair for bad key test
	badPubKey, badPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	badSigner, err := gossh.NewSignerFromKey(badPrivKey)
	require.NoError(t, err)
	sshBadPubKey, err := gossh.NewPublicKey(badPubKey)
	require.NoError(t, err)

	socketFile := filepath.Join(t.TempDir(), "beszel-test.sock")

	tests := []struct {
		name        string
		config      ServerOptions
		wantErr     bool
		errContains string
		setup       func() error
		cleanup     func() error
	}{
		{
			name: "tcp port only",
			config: ServerOptions{
				Network: "tcp",
				Addr:    ":45987",
				Keys:    []gossh.PublicKey{sshPubKey},
			},
		},
		{
			name: "tcp with ipv4",
			config: ServerOptions{
				Network: "tcp4",
				Addr:    "127.0.0.1:45988",
				Keys:    []gossh.PublicKey{sshPubKey},
			},
		},
		{
			name: "tcp with ipv6",
			config: ServerOptions{
				Network: "tcp6",
				Addr:    "[::1]:45989",
				Keys:    []gossh.PublicKey{sshPubKey},
			},
		},
		{
			name: "unix socket",
			config: ServerOptions{
				Network: "unix",
				Addr:    socketFile,
				Keys:    []gossh.PublicKey{sshPubKey},
			},
			setup: func() error {
				// Create a socket file that should be removed
				f, err := os.Create(socketFile)
				if err != nil {
					return err
				}
				return f.Close()
			},
			cleanup: func() error {
				return os.Remove(socketFile)
			},
		},
		{
			name: "bad key should fail",
			config: ServerOptions{
				Network: "tcp",
				Addr:    ":45987",
				Keys:    []gossh.PublicKey{sshBadPubKey},
			},
			wantErr:     true,
			errContains: "ssh: handshake failed",
		},
		{
			name: "good key still good",
			config: ServerOptions{
				Network: "tcp",
				Addr:    ":45987",
				Keys:    []gossh.PublicKey{sshPubKey},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				err := tt.setup()
				require.NoError(t, err)
			}

			if tt.cleanup != nil {
				defer tt.cleanup()
			}

			agent, err := NewAgent("")
			require.NoError(t, err)

			// Start server in a goroutine since it blocks
			errChan := make(chan error, 1)
			go func() {
				errChan <- agent.StartServer(tt.config)
			}()

			// Add a short delay to allow the server to start
			time.Sleep(100 * time.Millisecond)

			// Try to connect to verify server is running
			var client *gossh.Client

			// Choose the appropriate signer based on the test case
			testSigner := signer
			if tt.name == "bad key should fail" {
				testSigner = badSigner
			}

			sshClientConfig := &gossh.ClientConfig{
				User: "a",
				Auth: []gossh.AuthMethod{
					gossh.PublicKeys(testSigner),
				},
				HostKeyCallback: gossh.InsecureIgnoreHostKey(),
				Timeout:         4 * time.Second,
			}

			switch tt.config.Network {
			case "unix":
				client, err = gossh.Dial("unix", tt.config.Addr, sshClientConfig)
			default:
				if !strings.Contains(tt.config.Addr, ":") {
					tt.config.Addr = ":" + tt.config.Addr
				}
				client, err = gossh.Dial("tcp", tt.config.Addr, sshClientConfig)
			}

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)
			client.Close()
		})
	}
}

/////////////////////////////////////////////////////////////////
//////////////////// ParseKeys Tests ////////////////////////////
/////////////////////////////////////////////////////////////////

// Helper function to generate a temporary file with content
func createTempFile(content string) (string, error) {
	tmpFile, err := os.CreateTemp("", "ssh_keys_*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		return "", fmt.Errorf("failed to write to temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

// Test case 1: String with a single SSH key
func TestParseSingleKeyFromString(t *testing.T) {
	input := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKCBM91kukN7hbvFKtbpEeo2JXjCcNxXcdBH7V7ADMBo"
	keys, err := ParseKeys(input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d keys", len(keys))
	}
	if keys[0].Type() != "ssh-ed25519" {
		t.Fatalf("Expected key type 'ssh-ed25519', got '%s'", keys[0].Type())
	}
}

// Test case 2: String with multiple SSH keys
func TestParseMultipleKeysFromString(t *testing.T) {
	input := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKCBM91kukN7hbvFKtbpEeo2JXjCcNxXcdBH7V7ADMBo\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJDMtAOQfxDlCxe+A5lVbUY/DHxK1LAF2Z3AV0FYv36D \n #comment\n ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJDMtAOQfxDlCxe+A5lVbUY/DHxK1LAF2Z3AV0FYv36D"
	keys, err := ParseKeys(input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("Expected 3 keys, got %d keys", len(keys))
	}
	if keys[0].Type() != "ssh-ed25519" || keys[1].Type() != "ssh-ed25519" || keys[2].Type() != "ssh-ed25519" {
		t.Fatalf("Unexpected key types: %s, %s, %s", keys[0].Type(), keys[1].Type(), keys[2].Type())
	}
}

// Test case 3: File with a single SSH key
func TestParseSingleKeyFromFile(t *testing.T) {
	content := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKCBM91kukN7hbvFKtbpEeo2JXjCcNxXcdBH7V7ADMBo"
	filePath, err := createTempFile(content)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(filePath) // Clean up the file after the test

	// Read the file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	// Parse the keys
	keys, err := ParseKeys(string(fileContent))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d keys", len(keys))
	}
	if keys[0].Type() != "ssh-ed25519" {
		t.Fatalf("Expected key type 'ssh-ed25519', got '%s'", keys[0].Type())
	}
}

// Test case 4: File with multiple SSH keys
func TestParseMultipleKeysFromFile(t *testing.T) {
	content := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKCBM91kukN7hbvFKtbpEeo2JXjCcNxXcdBH7V7ADMBo\nssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJDMtAOQfxDlCxe+A5lVbUY/DHxK1LAF2Z3AV0FYv36D \n #comment\n ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJDMtAOQfxDlCxe+A5lVbUY/DHxK1LAF2Z3AV0FYv36D"
	filePath, err := createTempFile(content)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	// defer os.Remove(filePath) // Clean up the file after the test

	// Read the file content
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	// Parse the keys
	keys, err := ParseKeys(string(fileContent))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("Expected 3 keys, got %d keys", len(keys))
	}
	if keys[0].Type() != "ssh-ed25519" || keys[1].Type() != "ssh-ed25519" || keys[2].Type() != "ssh-ed25519" {
		t.Fatalf("Unexpected key types: %s, %s, %s", keys[0].Type(), keys[1].Type(), keys[2].Type())
	}
}

// Test case 5: Invalid SSH key input
func TestParseInvalidKey(t *testing.T) {
	input := "invalid-key-data"
	_, err := ParseKeys(input)
	if err == nil {
		t.Fatalf("Expected an error for invalid key, got nil")
	}
	expectedErrMsg := "failed to parse key"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Fatalf("Expected error message to contain '%s', got: %v", expectedErrMsg, err)
	}
}

/////////////////////////////////////////////////////////////////
//////////////////// Hub Version Tests //////////////////////////
/////////////////////////////////////////////////////////////////

func TestExtractHubVersion(t *testing.T) {
	tests := []struct {
		name            string
		clientVersion   string
		expectedVersion string
		expectError     bool
	}{
		{
			name:            "valid beszel client version with underscore",
			clientVersion:   "SSH-2.0-beszel_0.11.1",
			expectedVersion: "0.11.1",
			expectError:     false,
		},
		{
			name:            "valid beszel client version with beta",
			clientVersion:   "SSH-2.0-beszel_1.0.0-beta",
			expectedVersion: "1.0.0-beta",
			expectError:     false,
		},
		{
			name:            "valid beszel client version with rc",
			clientVersion:   "SSH-2.0-beszel_0.12.0-rc1",
			expectedVersion: "0.12.0-rc1",
			expectError:     false,
		},
		{
			name:            "different SSH client",
			clientVersion:   "SSH-2.0-OpenSSH_8.0",
			expectedVersion: "8.0",
			expectError:     true,
		},
		{
			name:          "malformed version string without underscore",
			clientVersion: "SSH-2.0-beszel",
			expectError:   true,
		},
		{
			name:          "empty version string",
			clientVersion: "",
			expectError:   true,
		},
		{
			name:            "version string with underscore but no version",
			clientVersion:   "beszel_",
			expectedVersion: "",
			expectError:     true,
		},
		{
			name:            "version with patch and build metadata",
			clientVersion:   "SSH-2.0-beszel_1.2.3+build.123",
			expectedVersion: "1.2.3+build.123",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractHubVersion(tt.clientVersion)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedVersion, result.String())
		})
	}
}

/////////////////////////////////////////////////////////////////
/////////////// Hub Version Detection Tests ////////////////////
/////////////////////////////////////////////////////////////////

func TestGetHubVersion(t *testing.T) {
	agent, err := NewAgent("")
	require.NoError(t, err)

	// Mock SSH context that implements the ssh.Context interface
	mockCtx := &mockSSHContext{
		sessionID:     "test-session-123",
		clientVersion: "SSH-2.0-beszel_0.12.0",
	}

	// Test first call - should extract and cache version
	version := agent.getHubVersion("test-session-123", mockCtx)
	assert.Equal(t, "0.12.0", version.String())

	// Test second call - should return cached version
	mockCtx.clientVersion = "SSH-2.0-beszel_0.11.0" // Change version but should still return cached
	version = agent.getHubVersion("test-session-123", mockCtx)
	assert.Equal(t, "0.12.0", version.String()) // Should still be cached version

	// Test different session - should extract new version
	version = agent.getHubVersion("different-session", mockCtx)
	assert.Equal(t, "0.11.0", version.String())

	// Test with invalid version string (non-beszel client)
	mockCtx.clientVersion = "SSH-2.0-OpenSSH_8.0"
	version = agent.getHubVersion("invalid-session", mockCtx)
	assert.Equal(t, "0.0.0", version.String()) // Should be empty version for non-beszel clients

	// Test with no client version
	mockCtx.clientVersion = ""
	version = agent.getHubVersion("no-version-session", mockCtx)
	assert.True(t, version.EQ(semver.Version{})) // Should be empty version
}

// mockSSHContext implements ssh.Context for testing
type mockSSHContext struct {
	context.Context
	sync.Mutex
	sessionID     string
	clientVersion string
}

func (m *mockSSHContext) SessionID() string {
	return m.sessionID
}

func (m *mockSSHContext) ClientVersion() string {
	return m.clientVersion
}

func (m *mockSSHContext) ServerVersion() string {
	return "SSH-2.0-beszel_test"
}

func (m *mockSSHContext) Value(key interface{}) interface{} {
	if key == ssh.ContextKeyClientVersion {
		return m.clientVersion
	}
	return nil
}

func (m *mockSSHContext) User() string                    { return "test-user" }
func (m *mockSSHContext) RemoteAddr() net.Addr            { return nil }
func (m *mockSSHContext) LocalAddr() net.Addr             { return nil }
func (m *mockSSHContext) Permissions() *ssh.Permissions   { return nil }
func (m *mockSSHContext) SetValue(key, value interface{}) {}

/////////////////////////////////////////////////////////////////
/////////////// CBOR vs JSON Encoding Tests ////////////////////
/////////////////////////////////////////////////////////////////

// TestWriteToSessionEncoding tests that writeToSession actually encodes data in the correct format
func TestWriteToSessionEncoding(t *testing.T) {
	tests := []struct {
		name             string
		hubVersion       string
		expectedUsesCbor bool
	}{
		{
			name:             "old hub version should use JSON",
			hubVersion:       "0.11.1",
			expectedUsesCbor: false,
		},
		{
			name:             "non-beta release should use CBOR",
			hubVersion:       "0.12.0",
			expectedUsesCbor: true,
		},
		{
			name:             "even newer hub version should use CBOR",
			hubVersion:       "0.16.4",
			expectedUsesCbor: true,
		},
		{
			name:             "beta version below release threshold should use JSON",
			hubVersion:       "0.12.0-beta0",
			expectedUsesCbor: false,
		},
		// {
		// 	name:             "matching beta version should use CBOR",
		// 	hubVersion:       "0.12.0-beta2",
		// 	expectedUsesCbor: true,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the global hubVersions map to ensure clean state for each test
			hubVersions = nil

			agent, err := NewAgent("")
			require.NoError(t, err)

			// Parse the test version
			version, err := semver.Parse(tt.hubVersion)
			require.NoError(t, err)

			// Create test data to encode
			testData := createTestCombinedData()

			var buf strings.Builder
			err = agent.writeToSession(&buf, testData, version)
			require.NoError(t, err)

			encodedData := buf.String()
			require.NotEmpty(t, encodedData)

			// Verify the encoding format by attempting to decode
			if tt.expectedUsesCbor {
				var decodedCbor system.CombinedData
				err = cbor.Unmarshal([]byte(encodedData), &decodedCbor)
				assert.NoError(t, err, "Should be valid CBOR data")

				var decodedJson system.CombinedData
				err = json.Unmarshal([]byte(encodedData), &decodedJson)
				assert.Error(t, err, "Should not be valid JSON data")

				assert.Equal(t, testData.Info.Hostname, decodedCbor.Info.Hostname)
				assert.Equal(t, testData.Stats.Cpu, decodedCbor.Stats.Cpu)
			} else {
				// Should be JSON - try to decode as JSON
				var decodedJson system.CombinedData
				err = json.Unmarshal([]byte(encodedData), &decodedJson)
				assert.NoError(t, err, "Should be valid JSON data")

				var decodedCbor system.CombinedData
				err = cbor.Unmarshal([]byte(encodedData), &decodedCbor)
				assert.Error(t, err, "Should not be valid CBOR data")

				// Verify the decoded JSON data matches our test data
				assert.Equal(t, testData.Info.Hostname, decodedJson.Info.Hostname)
				assert.Equal(t, testData.Stats.Cpu, decodedJson.Stats.Cpu)

				// Verify it looks like JSON (starts with '{' and contains readable field names)
				assert.True(t, strings.HasPrefix(encodedData, "{"), "JSON should start with '{'")
				assert.Contains(t, encodedData, `"info"`, "JSON should contain readable field names")
				assert.Contains(t, encodedData, `"stats"`, "JSON should contain readable field names")
			}
		})
	}
}

// Helper function to create test data for encoding tests
func createTestCombinedData() *system.CombinedData {
	return &system.CombinedData{
		Stats: system.Stats{
			Cpu:       25.5,
			Mem:       8589934592, // 8GB
			MemUsed:   4294967296, // 4GB
			MemPct:    50.0,
			DiskTotal: 1099511627776, // 1TB
			DiskUsed:  549755813888,  // 512GB
			DiskPct:   50.0,
		},
		Info: system.Info{
			Hostname:     "test-host",
			Cores:        8,
			CpuModel:     "Test CPU Model",
			Uptime:       3600,
			AgentVersion: "0.12.0",
			Os:           system.Linux,
		},
		Containers: []*container.Stats{
			{
				Name: "test-container",
				Cpu:  10.5,
				Mem:  1073741824, // 1GB
			},
		},
	}
}

func TestHubVersionCaching(t *testing.T) {
	// Reset the global hubVersions map to ensure clean state
	hubVersions = nil

	agent, err := NewAgent("")
	require.NoError(t, err)

	ctx1 := &mockSSHContext{
		sessionID:     "session1",
		clientVersion: "SSH-2.0-beszel_0.12.0",
	}
	ctx2 := &mockSSHContext{
		sessionID:     "session2",
		clientVersion: "SSH-2.0-beszel_0.11.0",
	}

	// First calls should cache the versions
	v1 := agent.getHubVersion("session1", ctx1)
	v2 := agent.getHubVersion("session2", ctx2)

	assert.Equal(t, "0.12.0", v1.String())
	assert.Equal(t, "0.11.0", v2.String())

	// Verify caching by changing context but keeping same session ID
	ctx1.clientVersion = "SSH-2.0-beszel_0.10.0"
	v1Cached := agent.getHubVersion("session1", ctx1)
	assert.Equal(t, "0.12.0", v1Cached.String()) // Should still be cached version

	// New session should get new version
	ctx3 := &mockSSHContext{
		sessionID:     "session3",
		clientVersion: "SSH-2.0-beszel_0.13.0",
	}
	v3 := agent.getHubVersion("session3", ctx3)
	assert.Equal(t, "0.13.0", v3.String())
}
