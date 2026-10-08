package ws

import (
	"context"
	"errors"

	"github.com/henrygd/beszel"

	"github.com/blang/semver"
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/lxzan/gws"
	"golang.org/x/crypto/ssh"
)

// ResponseHandler defines interface for handling agent responses.
// This is used by handleAgentRequest for legacy response handling.
type ResponseHandler interface {
	Handle(agentResponse common.AgentResponse) error
	HandleLegacy(rawData []byte) error
}

// BaseHandler provides a default implementation that can be embedded to make HandleLegacy optional
type BaseHandler struct{}

func (h *BaseHandler) HandleLegacy(rawData []byte) error {
	return errors.New("legacy format not supported")
}

////////////////////////////////////////////////////////////////////////////
// Fingerprint handling (used for WebSocket authentication)
////////////////////////////////////////////////////////////////////////////

// fingerprintHandler implements ResponseHandler for fingerprint requests
type fingerprintHandler struct {
	result *common.FingerprintResponse
}

func (h *fingerprintHandler) HandleLegacy(rawData []byte) error {
	return cbor.Unmarshal(rawData, h.result)
}

func (h *fingerprintHandler) Handle(agentResponse common.AgentResponse) error {
	if agentResponse.Fingerprint != nil {
		*h.result = *agentResponse.Fingerprint
		return nil
	}
	return errors.New("no fingerprint data in response")
}

// buildWsChallenge builds the signed challenge for the fingerprint check.
// For agents >= MinVersionWsNonce that supplied a connection nonce, the
// challenge is nonce||token where the nonce comes from the agent's own
// X-Agent-Nonce header - so the signature is bound to this connection and an
// observed handshake cannot be replayed by an impersonating hub. Agents
// without a nonce (legacy, or pre-0.22) get the bare-token challenge.
func buildWsChallenge(signer ssh.Signer, token, agentNonce string, agentVersion semver.Version) ([]byte, *ssh.Signature, error) {
	if agentVersion.GTE(beszel.MinVersionWsNonce) && agentNonce != "" {
		challenge := []byte(agentNonce + token)
		signature, err := signer.Sign(nil, challenge)
		if err != nil {
			return nil, nil, err
		}
		// Echo the agent's nonce so it knows to verify nonce||token.
		return []byte(agentNonce), signature, nil
	}
	signature, err := signer.Sign(nil, []byte(token))
	if err != nil {
		return nil, nil, err
	}
	// No echo: the agent falls back to the legacy bare-token verification.
	return nil, signature, nil
}

// GetFingerprint authenticates with the agent using SSH signature and returns the agent's fingerprint.
func (ws *WsConn) GetFingerprint(ctx context.Context, token string, signer ssh.Signer, needSysInfo bool) (common.FingerprintResponse, error) {
	if !ws.IsConnected() {
		return common.FingerprintResponse{}, gws.ErrConnClosed
	}

	// The returned challenge doubles as the echo: for the nonce protocol it
	// is the agent's own nonce, telling the agent which challenge to verify.
	challenge, signature, err := buildWsChallenge(signer, token, ws.agentNonce, ws.agentVersion)
	if err != nil {
		return common.FingerprintResponse{}, err
	}

	req, err := ws.requestManager.SendRequest(ctx, common.CheckFingerprint, common.FingerprintRequest{
		Signature:   signature.Blob,
		NeedSysInfo: needSysInfo,
		Nonce:       challenge,
	})
	if err != nil {
		return common.FingerprintResponse{}, err
	}

	var result common.FingerprintResponse
	handler := &fingerprintHandler{result: &result}
	err = ws.handleAgentRequest(req, handler)
	return result, err
}
