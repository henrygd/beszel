// Package beszel provides core application constants and version information
// which are used throughout the application.
package beszel

import "github.com/blang/semver"

const (
	// Version is the current version of the application.
	Version = "0.21.0"
	// AppName is the name of the application.
	AppName = "beszel"
)

// MinVersionCbor is the minimum supported version for CBOR compatibility.
var MinVersionCbor = semver.MustParse("0.12.0")

// MinVersionAgentResponse is the minimum supported version for AgentResponse compatibility.
var MinVersionAgentResponse = semver.MustParse("0.13.0")

// MinVersionZfsData is the minimum agent version that supports ZFS detail requests.
var MinVersionZfsData = semver.MustParse("0.18.9")

// MinVersionNetworkMonitors is the minimum agent version that supports network monitor sync.
var MinVersionNetworkMonitors = semver.MustParse("0.20.0")

// MinVersionStableHostKey is the minimum agent version that persists a stable
// SSH host key, so the hub can pin it (TOFU) instead of ignoring host keys.
var MinVersionStableHostKey = semver.MustParse("0.22.0")

// MinVersionWsNonce is the minimum agent version that verifies the WebSocket
// fingerprint challenge against a per-connection nonce, making the hub's
// signature replay-proof.
var MinVersionWsNonce = semver.MustParse("0.22.0")
