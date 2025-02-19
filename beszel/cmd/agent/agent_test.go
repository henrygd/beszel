package main

import (
	"crypto/ed25519"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestGetAddress(t *testing.T) {
	tests := []struct {
		name     string
		cfg      cmdConfig
		envVars  map[string]string
		expected string
	}{
		{
			name:     "default port when no config",
			cfg:      cmdConfig{},
			expected: ":45876",
		},
		{
			name: "use address from flag",
			cfg: cmdConfig{
				addr: "8080",
			},
			expected: "8080",
		},
		{
			name: "use unix socket from flag",
			cfg: cmdConfig{
				addr: "/tmp/beszel.sock",
			},
			expected: "/tmp/beszel.sock",
		},
		{
			name: "use ADDR env var",
			cfg:  cmdConfig{},
			envVars: map[string]string{
				"ADDR": "1.2.3.4:9090",
			},
			expected: "1.2.3.4:9090",
		},
		{
			name: "use legacy PORT env var",
			cfg:  cmdConfig{},
			envVars: map[string]string{
				"PORT": "7070",
			},
			expected: "7070",
		},
		{
			name: "flag takes precedence over env vars",
			cfg: cmdConfig{
				addr: ":8080",
			},
			envVars: map[string]string{
				"ADDR": ":9090",
				"PORT": "7070",
			},
			expected: ":8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			addr := getAddress(tt.cfg.addr)
			assert.Equal(t, tt.expected, addr)
		})
	}
}

func TestLoadPublicKeys(t *testing.T) {
	// Generate a test key
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	pubKey := ssh.MarshalAuthorizedKey(signer.PublicKey())

	tests := []struct {
		name        string
		cfg         cmdConfig
		envVars     map[string]string
		setupFiles  map[string][]byte
		wantErr     bool
		errContains string
	}{
		{
			name: "load key from flag",
			cfg: cmdConfig{
				key: string(pubKey),
			},
		},
		{
			name: "load key from env var",
			envVars: map[string]string{
				"KEY": string(pubKey),
			},
		},
		{
			name: "load key from file",
			envVars: map[string]string{
				"KEY_FILE": "testkey.pub",
			},
			setupFiles: map[string][]byte{
				"testkey.pub": pubKey,
			},
		},
		{
			name:        "error when no key provided",
			wantErr:     true,
			errContains: "no key provided",
		},
		{
			name: "error on invalid key file",
			envVars: map[string]string{
				"KEY_FILE": "nonexistent.pub",
			},
			wantErr:     true,
			errContains: "failed to read key file",
		},
		{
			name: "error on invalid key data",
			cfg: cmdConfig{
				key: "invalid-key-data",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for test files
			if len(tt.setupFiles) > 0 {
				tmpDir := t.TempDir()
				for name, content := range tt.setupFiles {
					path := filepath.Join(tmpDir, name)
					err := os.WriteFile(path, content, 0600)
					require.NoError(t, err)
					if tt.envVars != nil {
						tt.envVars["KEY_FILE"] = path
					}
				}
			}

			// Set up environment
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			keys, err := loadPublicKeys(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, keys, 1)
			assert.Equal(t, signer.PublicKey().Type(), keys[0].Type())
		})
	}
}

func TestGetNetwork(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "only port",
			addr:     "8080",
			expected: "tcp",
		},
		{
			name:     "ipv4 address",
			addr:     "1.2.3.4:8080",
			expected: "tcp",
		},
		{
			name:     "ipv6 address",
			addr:     "[2001:db8::1]:8080",
			expected: "tcp",
		},
		{
			name:     "unix network",
			addr:     "/tmp/beszel.sock",
			expected: "unix",
		},
		{
			name:     "env var network",
			addr:     ":8080",
			envVars:  map[string]string{"NETWORK": "tcp4"},
			expected: "tcp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			network := getNetwork(tt.addr)
			assert.Equal(t, tt.expected, network)
		})
	}
}

func TestParseFlags(t *testing.T) {
	// Save original command line arguments and restore after test
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	}()

	tests := []struct {
		name     string
		args     []string
		expected cmdConfig
	}{
		{
			name: "no flags",
			args: []string{"cmd"},
			expected: cmdConfig{
				key:  "",
				addr: "",
			},
		},
		{
			name: "key flag only",
			args: []string{"cmd", "-key", "testkey"},
			expected: cmdConfig{
				key:  "testkey",
				addr: "",
			},
		},
		{
			name: "addr flag only",
			args: []string{"cmd", "-addr", ":8080"},
			expected: cmdConfig{
				key:  "",
				addr: ":8080",
			},
		},
		{
			name: "both flags",
			args: []string{"cmd", "-key", "testkey", "-addr", ":8080"},
			expected: cmdConfig{
				key:  "testkey",
				addr: ":8080",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags for each test
			flag.CommandLine = flag.NewFlagSet(tt.args[0], flag.ExitOnError)
			os.Args = tt.args

			var cfg cmdConfig
			parseFlags(&cfg)
			flag.Parse()

			assert.Equal(t, tt.expected, cfg)
		})
	}
}
