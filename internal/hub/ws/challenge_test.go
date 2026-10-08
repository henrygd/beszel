//go:build testing

package ws

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/henrygd/beszel"

	"github.com/blang/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// TestBuildWsChallenge pins the fingerprint-challenge contract: agents
// >= MinVersionWsNonce that supplied a connection nonce get a signature over
// nonce||token (with the nonce echoed back so the agent knows which challenge
// to verify), while agents without a nonce get the bare-token challenge.
// The nonce comes from the AGENT's connection header: because the verifying
// side contributes the freshness, a signature captured on one connection is
// useless on another (whose nonce differs).
func TestBuildWsChallenge(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(privKey)
	require.NoError(t, err)

	const token = "test-token"
	const agentNonce = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	t.Run("new agent with nonce gets it bound and echoed", func(t *testing.T) {
		echo, sig, err := buildWsChallenge(signer, token, agentNonce, beszel.MinVersionWsNonce)
		require.NoError(t, err)
		require.Equal(t, []byte(agentNonce), echo, "agent's nonce must be echoed back")

		// The signature verifies over nonce||token...
		message := append([]byte(agentNonce), token...)
		assert.NoError(t, signer.PublicKey().Verify(message, sig))
		// ...and NOT over the bare token (replaying a legacy handshake).
		assert.Error(t, signer.PublicKey().Verify([]byte(token), sig))
	})

	t.Run("new agent without a header nonce falls back to legacy challenge", func(t *testing.T) {
		echo, sig, err := buildWsChallenge(signer, token, "", beszel.MinVersionWsNonce)
		require.NoError(t, err)
		assert.Empty(t, echo)
		assert.NoError(t, signer.PublicKey().Verify([]byte(token), sig))
	})

	t.Run("legacy agent gets bare-token challenge", func(t *testing.T) {
		legacy := semver.MustParse("0.13.0")
		echo, sig, err := buildWsChallenge(signer, token, agentNonce, legacy)
		require.NoError(t, err)
		assert.Empty(t, echo, "nonce must not be echoed to legacy agents")
		assert.NoError(t, signer.PublicKey().Verify([]byte(token), sig))
	})
}
