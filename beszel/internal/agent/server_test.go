package agent

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestStartServer(t *testing.T) {
	// Generate a test key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(privKey)
	require.NoError(t, err)
	sshPubKey, err := ssh.NewPublicKey(pubKey)
	require.NoError(t, err)

	// Generate a different key pair for bad key test
	badPubKey, badPrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	badSigner, err := ssh.NewSignerFromKey(badPrivKey)
	require.NoError(t, err)
	sshBadPubKey, err := ssh.NewPublicKey(badPubKey)
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
				Keys:    []ssh.PublicKey{sshPubKey},
			},
		},
		{
			name: "tcp with ipv4",
			config: ServerOptions{
				Network: "tcp4",
				Addr:    "127.0.0.1:45988",
				Keys:    []ssh.PublicKey{sshPubKey},
			},
		},
		{
			name: "tcp with ipv6",
			config: ServerOptions{
				Network: "tcp6",
				Addr:    "[::1]:45989",
				Keys:    []ssh.PublicKey{sshPubKey},
			},
		},
		{
			name: "unix socket",
			config: ServerOptions{
				Network: "unix",
				Addr:    socketFile,
				Keys:    []ssh.PublicKey{sshPubKey},
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
				Keys:    []ssh.PublicKey{sshBadPubKey},
			},
			wantErr:     true,
			errContains: "ssh: handshake failed",
		},
		{
			name: "good key still good",
			config: ServerOptions{
				Network: "tcp",
				Addr:    ":45987",
				Keys:    []ssh.PublicKey{sshPubKey},
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

			agent := NewAgent()

			// Start server in a goroutine since it blocks
			errChan := make(chan error, 1)
			go func() {
				errChan <- agent.StartServer(tt.config)
			}()

			// Add a short delay to allow the server to start
			time.Sleep(100 * time.Millisecond)

			// Try to connect to verify server is running
			var client *ssh.Client
			var err error

			// Choose the appropriate signer based on the test case
			testSigner := signer
			if tt.name == "bad key should fail" {
				testSigner = badSigner
			}

			sshClientConfig := &ssh.ClientConfig{
				User: "a",
				Auth: []ssh.AuthMethod{
					ssh.PublicKeys(testSigner),
				},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         4 * time.Second,
			}

			switch tt.config.Network {
			case "unix":
				client, err = ssh.Dial("unix", tt.config.Addr, sshClientConfig)
			default:
				if !strings.Contains(tt.config.Addr, ":") {
					tt.config.Addr = ":" + tt.config.Addr
				}
				client, err = ssh.Dial("tcp", tt.config.Addr, sshClientConfig)
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
