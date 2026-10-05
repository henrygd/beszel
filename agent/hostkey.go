package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

// hostKeyFileName is the file (under the agent data directory) that persists
// the agent's SSH host key, so the hub can pin it instead of treating every
// restart as a new host.
const hostKeyFileName = "host_key"

// getHostKeySigner loads the persisted SSH host key, generating and saving one
// on first use. Without a data directory the agent falls back to an ephemeral
// key (previous behavior); the hub only pins host keys for agents that
// advertise a beszel version with stable keys.
func (a *Agent) getHostKeySigner() (gossh.Signer, error) {
	if a.dataDir == "" {
		return nil, errors.New("no data directory; cannot persist host key")
	}
	keyPath := filepath.Join(a.dataDir, hostKeyFileName)

	if keyBytes, err := os.ReadFile(keyPath); err == nil {
		return gossh.ParsePrivateKey(keyBytes)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	pemBlock, err := gossh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		return nil, err
	}
	signer, err := gossh.NewSignerFromKey(privKey)
	if err != nil {
		return nil, err
	}
	slog.Info("SSH host key generated", "path", keyPath)
	return signer, nil
}
