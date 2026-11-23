//go:build testing

package hub

import (
	"github.com/henrygd/beszel/internal/hub/systems"
)

// TESTING ONLY: GetSystemManager returns the system manager
func (h *Hub) GetSystemManager() *systems.SystemManager {
	return h.sm
}

// TESTING ONLY: GetPubkey returns the public key
func (h *Hub) GetPubkey() string {
	h.sshKeyMu.Lock()
	defer h.sshKeyMu.Unlock()
	return h.pubKey
}

// TESTING ONLY: SetPubkey sets the public key
func (h *Hub) SetPubkey(pubkey string) {
	h.sshKeyMu.Lock()
	defer h.sshKeyMu.Unlock()
	h.pubKey = pubkey
}

// TESTING ONLY: ResetSSHKey clears the cached signer and public key
func (h *Hub) ResetSSHKey() {
	h.sshKeyMu.Lock()
	defer h.sshKeyMu.Unlock()
	h.signer = nil
	h.pubKey = ""
}

func (h *Hub) SetCollectionAuthSettings() error {
	return setCollectionAuthSettings(h)
}
