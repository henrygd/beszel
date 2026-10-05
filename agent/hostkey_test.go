//go:build testing

package agent

import (
	"crypto/ed25519"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

// TestHostKeyPersistence pins the host key contract the hub's TOFU pinning
// depends on: the key is stored in the data directory (mode 0600), is reused
// across agent restarts, and is absent when no data directory is configured
// (the agent then uses an ephemeral key, as before 0.22).
func TestHostKeyPersistence(t *testing.T) {
	dataDir := t.TempDir()
	a1, err := NewAgent(dataDir)
	require.NoError(t, err)
	signer1, err := a1.getHostKeySigner()
	require.NoError(t, err)

	a2, err := NewAgent(dataDir)
	require.NoError(t, err)
	signer2, err := a2.getHostKeySigner()
	require.NoError(t, err)

	assert.Equal(t,
		gossh.FingerprintSHA256(signer1.PublicKey()),
		gossh.FingerprintSHA256(signer2.PublicKey()),
		"host key must be stable across agent restarts")

	info, err := os.Stat(filepath.Join(dataDir, hostKeyFileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(),
		"host key file must not be world-readable")

	// No data directory: signer cannot be created; prepareSSHServer falls
	// back to an ephemeral key.
	a3, err := NewAgent("")
	require.NoError(t, err)
	_, err = a3.getHostKeySigner()
	assert.Error(t, err)
}

// TestHostKeyStableAcrossRestarts verifies end to end that two agent
// processes sharing a data directory present the identical host key over the
// wire — the property a TOFU-pinning hub relies on.
func TestHostKeyStableAcrossRestarts(t *testing.T) {
	dataDir := t.TempDir()

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(privKey)
	require.NoError(t, err)
	sshPubKey, err := gossh.NewPublicKey(pubKey)
	require.NoError(t, err)

	var fingerprints []string
	for i := 0; i < 2; i++ {
		agent, err := NewAgent(dataDir)
		require.NoError(t, err)

		socketFile := filepath.Join(t.TempDir(), "beszel-test.sock")
		go func() {
			_ = agent.StartServer(ServerOptions{Network: "unix", Addr: socketFile, Keys: []gossh.PublicKey{sshPubKey}})
		}()
		require.Eventually(t, func() bool {
			conn, err := net.Dial("unix", socketFile)
			if err != nil {
				return false
			}
			_ = conn.Close()
			return true
		}, 5*time.Second, 25*time.Millisecond)

		client, err := gossh.Dial("unix", socketFile, &gossh.ClientConfig{
			User: "u",
			Auth: []gossh.AuthMethod{gossh.PublicKeys(signer)},
			HostKeyCallback: func(hostname string, remote net.Addr, key gossh.PublicKey) error {
				fingerprints = append(fingerprints, gossh.FingerprintSHA256(key))
				return nil
			},
			Timeout: 4 * time.Second,
		})
		require.NoError(t, err)
		require.NoError(t, client.Close())
		require.NoError(t, agent.StopServer())
	}

	require.Len(t, fingerprints, 2)
	assert.Equal(t, fingerprints[0], fingerprints[1],
		"host key fingerprint must be identical across agent restarts")
}
