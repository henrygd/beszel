//go:build testing
// +build testing

package hub_test

import (
	"beszel/internal/tests"
	"testing"

	"crypto/ed25519"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func getTestHub(t testing.TB) *tests.TestHub {
	hub, _ := tests.NewTestHub(t.TempDir())
	return hub
}

func TestMakeLink(t *testing.T) {
	hub := getTestHub(t)

	tests := []struct {
		name     string
		appURL   string
		parts    []string
		expected string
	}{
		{
			name:     "no parts, no trailing slash in AppURL",
			appURL:   "http://localhost:8090",
			parts:    []string{},
			expected: "http://localhost:8090",
		},
		{
			name:     "no parts, with trailing slash in AppURL",
			appURL:   "http://localhost:8090/",
			parts:    []string{},
			expected: "http://localhost:8090", // TrimSuffix should handle the trailing slash
		},
		{
			name:     "one part",
			appURL:   "http://example.com",
			parts:    []string{"one"},
			expected: "http://example.com/one",
		},
		{
			name:     "multiple parts",
			appURL:   "http://example.com",
			parts:    []string{"alpha", "beta", "gamma"},
			expected: "http://example.com/alpha/beta/gamma",
		},
		{
			name:     "parts with spaces needing escaping",
			appURL:   "http://example.com",
			parts:    []string{"path with spaces", "another part"},
			expected: "http://example.com/path%20with%20spaces/another%20part",
		},
		{
			name:     "parts with slashes needing escaping",
			appURL:   "http://example.com",
			parts:    []string{"a/b", "c"},
			expected: "http://example.com/a%2Fb/c", // url.PathEscape escapes '/'
		},
		{
			name:     "AppURL with subpath, no trailing slash",
			appURL:   "http://localhost/sub",
			parts:    []string{"resource"},
			expected: "http://localhost/sub/resource",
		},
		{
			name:     "AppURL with subpath, with trailing slash",
			appURL:   "http://localhost/sub/",
			parts:    []string{"item"},
			expected: "http://localhost/sub/item",
		},
		{
			name:     "empty parts in the middle",
			appURL:   "http://localhost",
			parts:    []string{"first", "", "third"},
			expected: "http://localhost/first/third",
		},
		{
			name:     "leading and trailing empty parts",
			appURL:   "http://localhost",
			parts:    []string{"", "path", ""},
			expected: "http://localhost/path",
		},
		{
			name:     "parts with various special characters",
			appURL:   "https://test.dev/",
			parts:    []string{"p@th?", "key=value&"},
			expected: "https://test.dev/p@th%3F/key=value&",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original app URL and restore it after the test
			originalAppURL := hub.Settings().Meta.AppURL
			hub.Settings().Meta.AppURL = tt.appURL
			defer func() { hub.Settings().Meta.AppURL = originalAppURL }()

			got := hub.MakeLink(tt.parts...)
			assert.Equal(t, tt.expected, got, "MakeLink generated URL does not match expected")
		})
	}
}

func TestGetSSHKey(t *testing.T) {
	hub := getTestHub(t)

	// Test Case 1: Key generation (no existing key)
	t.Run("KeyGeneration", func(t *testing.T) {
		tempDir := t.TempDir()

		// Ensure pubKey is initially empty or different to ensure GetSSHKey sets it
		hub.SetPubkey("")

		signer, err := hub.GetSSHKey(tempDir)
		assert.NoError(t, err, "GetSSHKey should not error when generating a new key")
		assert.NotNil(t, signer, "GetSSHKey should return a non-nil signer")

		// Check if private key file was created
		privateKeyPath := filepath.Join(tempDir, "id_ed25519")
		info, err := os.Stat(privateKeyPath)
		assert.NoError(t, err, "Private key file should be created")
		assert.False(t, info.IsDir(), "Private key path should be a file, not a directory")

		// Check if h.pubKey was set
		assert.NotEmpty(t, hub.GetPubkey(), "h.pubKey should be set after key generation")
		assert.True(t, strings.HasPrefix(hub.GetPubkey(), "ssh-ed25519 "), "h.pubKey should start with 'ssh-ed25519 '")

		// Verify the generated private key is parsable
		keyData, err := os.ReadFile(privateKeyPath)
		require.NoError(t, err)
		_, err = ssh.ParsePrivateKey(keyData)
		assert.NoError(t, err, "Generated private key should be parsable by ssh.ParsePrivateKey")
	})

	// Test Case 2: Existing key
	t.Run("ExistingKey", func(t *testing.T) {
		tempDir := t.TempDir()

		// Manually create a valid key pair for the test
		rawPubKey, rawPrivKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err, "Failed to generate raw ed25519 key pair for pre-existing key test")

		// Marshal the private key into OpenSSH PEM format
		pemBlock, err := ssh.MarshalPrivateKey(rawPrivKey, "")
		require.NoError(t, err, "Failed to marshal private key to PEM block for pre-existing key test")

		privateKeyBytes := pem.EncodeToMemory(pemBlock)
		require.NotNil(t, privateKeyBytes, "PEM encoded private key bytes should not be nil")

		privateKeyPath := filepath.Join(tempDir, "id_ed25519")
		err = os.WriteFile(privateKeyPath, privateKeyBytes, 0600)
		require.NoError(t, err, "Failed to write pre-existing private key")

		// Determine the expected public key string
		sshPubKey, err := ssh.NewPublicKey(rawPubKey)
		require.NoError(t, err)
		expectedPubKeyStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPubKey)))

		// Reset h.pubKey to ensure it's set by GetSSHKey from the file
		hub.SetPubkey("")

		signer, err := hub.GetSSHKey(tempDir)
		assert.NoError(t, err, "GetSSHKey should not error when reading an existing key")
		assert.NotNil(t, signer, "GetSSHKey should return a non-nil signer for an existing key")

		// Check if h.pubKey was set correctly to the public key from the file
		assert.Equal(t, expectedPubKeyStr, hub.GetPubkey(), "h.pubKey should match the existing public key")

		// Verify the signer's public key matches the original public key
		signerPubKey := signer.PublicKey()
		marshaledSignerPubKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signerPubKey)))
		assert.Equal(t, expectedPubKeyStr, marshaledSignerPubKey, "Signer's public key should match the existing public key")
	})

	// Test Case 3: Error cases
	t.Run("ErrorCases", func(t *testing.T) {
		tests := []struct {
			name       string
			setupFunc  func(dir string) error
			errorCheck func(t *testing.T, err error)
		}{
			{
				name: "CorruptedKey",
				setupFunc: func(dir string) error {
					return os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("this is not a valid SSH key"), 0600)
				},
				errorCheck: func(t *testing.T, err error) {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "ssh: no key found")
				},
			},
			{
				name: "PermissionDenied",
				setupFunc: func(dir string) error {
					// Create the key file
					keyPath := filepath.Join(dir, "id_ed25519")
					if err := os.WriteFile(keyPath, []byte("dummy content"), 0600); err != nil {
						return err
					}
					// Make it read-only (can't be opened for writing in case a new key needs to be written)
					return os.Chmod(keyPath, 0400)
				},
				errorCheck: func(t *testing.T, err error) {
					// On read-only key, the parser will attempt to parse it and fail with "ssh: no key found"
					assert.Error(t, err)
				},
			},
			{
				name: "EmptyFile",
				setupFunc: func(dir string) error {
					// Create an empty file
					return os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte{}, 0600)
				},
				errorCheck: func(t *testing.T, err error) {
					assert.Error(t, err)
					// The error from attempting to parse an empty file
					assert.Contains(t, err.Error(), "ssh: no key found")
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				tempDir := t.TempDir()

				// Setup the test case
				err := tc.setupFunc(tempDir)
				require.NoError(t, err, "Setup failed")

				// Reset h.pubKey before each test case
				hub.SetPubkey("")

				// Attempt to get SSH key
				_, err = hub.GetSSHKey(tempDir)

				// Verify the error
				tc.errorCheck(t, err)

				// Check that pubKey was not set in error cases
				assert.Empty(t, hub.GetPubkey(), "h.pubKey should not be set if there was an error")
			})
		}
	})
}

// Helper function to create test records
