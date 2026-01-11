package hub

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/hub/expirymap"
	"github.com/henrygd/beszel/internal/hub/ws"

	"github.com/blang/semver"
	"github.com/lxzan/gws"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// agentConnectRequest holds information related to an agent's connection attempt.
type agentConnectRequest struct {
	hub         *Hub
	req         *http.Request
	res         http.ResponseWriter
	token       string
	agentSemVer semver.Version
	// isUniversalToken is true if the token is a universal token.
	isUniversalToken bool
	// userId is the user ID associated with the universal token.
	userId string
}

// universalTokenMap stores active universal tokens and their associated user IDs.
var universalTokenMap tokenMap

type tokenMap struct {
	store *expirymap.ExpiryMap[string]
	once  sync.Once
}

// getMap returns the expirymap, creating it if necessary.
func (tm *tokenMap) GetMap() *expirymap.ExpiryMap[string] {
	tm.once.Do(func() {
		tm.store = expirymap.New[string](time.Hour)
	})
	return tm.store
}

// handleAgentConnect is the HTTP handler for an agent's connection request.
func (h *Hub) handleAgentConnect(e *core.RequestEvent) error {
	agentRequest := agentConnectRequest{req: e.Request, res: e.Response, hub: h}
	_ = agentRequest.agentConnect()
	return nil
}

// agentConnect validates agent credentials and upgrades the connection to a WebSocket.
func (acr *agentConnectRequest) agentConnect() (err error) {
	var agentVersion string

	acr.token, agentVersion, err = acr.validateAgentHeaders(acr.req.Header)
	if err != nil {
		return acr.sendResponseError(acr.res, http.StatusBadRequest, "")
	}

	// Check if token is an active universal token
	acr.userId, acr.isUniversalToken = universalTokenMap.GetMap().GetOk(acr.token)
	if !acr.isUniversalToken {
		// Fallback: check for a permanent universal token stored in the DB
		if rec, err := acr.hub.FindFirstRecordByFilter("universal_tokens", "token = {:token}", dbx.Params{"token": acr.token}); err == nil {
			if userID := rec.GetString("user"); userID != "" {
				acr.userId = userID
				acr.isUniversalToken = true
			}
		}
	}

	// Find matching fingerprint records for this token
	fpRecords := getFingerprintRecordsByToken(acr.token, acr.hub)
	if len(fpRecords) == 0 && !acr.isUniversalToken {
		// Invalid token - no records found and not a universal token
		return acr.sendResponseError(acr.res, http.StatusUnauthorized, "Invalid token")
	}

	// Validate agent version
	acr.agentSemVer, err = semver.Parse(agentVersion)
	if err != nil {
		return acr.sendResponseError(acr.res, http.StatusUnauthorized, "Invalid agent version")
	}

	// Upgrade connection to WebSocket
	conn, err := ws.GetUpgrader().Upgrade(acr.res, acr.req)
	if err != nil {
		return acr.sendResponseError(acr.res, http.StatusInternalServerError, "WebSocket upgrade failed")
	}

	go acr.verifyWsConn(conn, fpRecords)

	return nil
}

// verifyWsConn verifies the WebSocket connection using the agent's fingerprint and
// SSH key signature, then adds the system to the system manager.
func (acr *agentConnectRequest) verifyWsConn(conn *gws.Conn, fpRecords []ws.FingerprintRecord) (err error) {
	wsConn := ws.NewWsConnection(conn, acr.agentSemVer)

	// must set wsConn in connection store before the read loop
	conn.Session().Store("wsConn", wsConn)

	// make sure connection is closed if there is an error
	defer func() {
		if err != nil {
			wsConn.Close([]byte(err.Error()))
		}
	}()

	go conn.ReadLoop()

	signer, err := acr.hub.GetSSHKey("")
	if err != nil {
		return err
	}

	agentFingerprint, err := wsConn.GetFingerprint(context.Background(), acr.token, signer, acr.isUniversalToken)
	if err != nil {
		return err
	}

	// Find or create the appropriate system for this token and fingerprint
	fpRecord, err := acr.findOrCreateSystemForToken(fpRecords, agentFingerprint)
	if err != nil {
		return err
	}

	return acr.hub.sm.AddWebSocketSystem(fpRecord.SystemId, acr.agentSemVer, wsConn)
}

// validateAgentHeaders extracts and validates the token and agent version from HTTP headers.
func (acr *agentConnectRequest) validateAgentHeaders(headers http.Header) (string, string, error) {
	token := headers.Get("X-Token")
	agentVersion := headers.Get("X-Beszel")

	if agentVersion == "" || token == "" || len(token) > 64 {
		return "", "", errors.New("")
	}
	return token, agentVersion, nil
}

// sendResponseError writes an HTTP error response.
func (acr *agentConnectRequest) sendResponseError(res http.ResponseWriter, code int, message string) error {
	res.WriteHeader(code)
	if message != "" {
		res.Write([]byte(message))
	}
	return nil
}

// getFingerprintRecordsByToken retrieves all fingerprint records associated with a given token.
func getFingerprintRecordsByToken(token string, h *Hub) []ws.FingerprintRecord {
	var records []ws.FingerprintRecord
	// All will populate empty slice even on error
	_ = h.DB().NewQuery("SELECT id, system, fingerprint, token FROM fingerprints WHERE token = {:token}").
		Bind(dbx.Params{
			"token": token,
		}).
		All(&records)
	return records
}

// findOrCreateSystemForToken finds an existing system matching the token and fingerprint,
// or creates a new one for a universal token.
func (acr *agentConnectRequest) findOrCreateSystemForToken(fpRecords []ws.FingerprintRecord, agentFingerprint common.FingerprintResponse) (ws.FingerprintRecord, error) {
	// No records - only valid for active universal tokens
	if len(fpRecords) == 0 {
		return acr.handleNoRecords(agentFingerprint)
	}

	// Single record - handle as regular token
	if len(fpRecords) == 1 && !acr.isUniversalToken {
		return acr.handleSingleRecord(fpRecords[0], agentFingerprint)
	}

	// Multiple records or universal token - look for matching fingerprint
	return acr.handleMultipleRecordsOrUniversalToken(fpRecords, agentFingerprint)
}

// handleNoRecords handles the case where no fingerprint records are found for a token.
// A new system is created if the token is a valid universal token.
func (acr *agentConnectRequest) handleNoRecords(agentFingerprint common.FingerprintResponse) (ws.FingerprintRecord, error) {
	var fpRecord ws.FingerprintRecord

	if !acr.isUniversalToken || acr.userId == "" {
		return fpRecord, errors.New("no matching fingerprints")
	}

	return acr.createNewSystemForUniversalToken(agentFingerprint)
}

// handleSingleRecord handles the case with a single fingerprint record. It validates
// the agent's fingerprint against the stored one, or sets it on first connect.
func (acr *agentConnectRequest) handleSingleRecord(fpRecord ws.FingerprintRecord, agentFingerprint common.FingerprintResponse) (ws.FingerprintRecord, error) {
	// If no current fingerprint, update with new fingerprint (first time connecting)
	if fpRecord.Fingerprint == "" {
		if err := acr.hub.SetFingerprint(&fpRecord, agentFingerprint.Fingerprint); err != nil {
			return fpRecord, err
		}
		// Update the record with the fingerprint that was set
		fpRecord.Fingerprint = agentFingerprint.Fingerprint
		return fpRecord, nil
	}

	// Abort if fingerprint exists but doesn't match (different machine)
	if fpRecord.Fingerprint != agentFingerprint.Fingerprint {
		return fpRecord, errors.New("fingerprint mismatch")
	}

	return fpRecord, nil
}

// handleMultipleRecordsOrUniversalToken finds a matching fingerprint from multiple records.
// If no match is found and the token is a universal token, a new system is created.
func (acr *agentConnectRequest) handleMultipleRecordsOrUniversalToken(fpRecords []ws.FingerprintRecord, agentFingerprint common.FingerprintResponse) (ws.FingerprintRecord, error) {
	// Return existing record with matching fingerprint if found
	for i := range fpRecords {
		if fpRecords[i].Fingerprint == agentFingerprint.Fingerprint {
			return fpRecords[i], nil
		}
	}

	// No matching fingerprint record found, but it's
	// an active universal token so create a new system
	if acr.isUniversalToken {
		return acr.createNewSystemForUniversalToken(agentFingerprint)
	}

	return ws.FingerprintRecord{}, errors.New("fingerprint mismatch")
}

// createNewSystemForUniversalToken creates a new system and fingerprint record for a universal token.
func (acr *agentConnectRequest) createNewSystemForUniversalToken(agentFingerprint common.FingerprintResponse) (ws.FingerprintRecord, error) {
	var fpRecord ws.FingerprintRecord
	if !acr.isUniversalToken || acr.userId == "" {
		return fpRecord, errors.New("invalid token")
	}

	fpRecord.Token = acr.token

	systemId, err := acr.createSystem(agentFingerprint)
	if err != nil {
		return fpRecord, err
	}
	fpRecord.SystemId = systemId

	// Set the fingerprint for the new system
	if err := acr.hub.SetFingerprint(&fpRecord, agentFingerprint.Fingerprint); err != nil {
		return fpRecord, err
	}

	// Update the record with the fingerprint that was set
	fpRecord.Fingerprint = agentFingerprint.Fingerprint

	return fpRecord, nil
}

// createSystem creates a new system record in the database using details from the agent.
func (acr *agentConnectRequest) createSystem(agentFingerprint common.FingerprintResponse) (recordId string, err error) {
	systemsCollection, err := acr.hub.FindCachedCollectionByNameOrId("systems")
	if err != nil {
		return "", err
	}
	remoteAddr := getRealIP(acr.req)
	// separate port from address
	if agentFingerprint.Hostname == "" {
		agentFingerprint.Hostname = remoteAddr
	}
	if agentFingerprint.Port == "" {
		agentFingerprint.Port = "45876"
	}
	if agentFingerprint.Name == "" {
		agentFingerprint.Name = agentFingerprint.Hostname
	}
	// create new record
	systemRecord := core.NewRecord(systemsCollection)
	systemRecord.Set("name", agentFingerprint.Name)
	systemRecord.Set("host", remoteAddr)
	systemRecord.Set("port", agentFingerprint.Port)
	systemRecord.Set("users", []string{acr.userId})

	return systemRecord.Id, acr.hub.Save(systemRecord)
}

// SetFingerprint creates or updates a fingerprint record in the database.
func (h *Hub) SetFingerprint(fpRecord *ws.FingerprintRecord, fingerprint string) (err error) {
	// // can't use raw query here because it doesn't trigger SSE
	var record *core.Record
	switch fpRecord.Id {
	case "":
		// create new record for universal token
		collection, _ := h.FindCachedCollectionByNameOrId("fingerprints")
		record = core.NewRecord(collection)
		record.Set("system", fpRecord.SystemId)
	default:
		record, err = h.FindRecordById("fingerprints", fpRecord.Id)
	}
	if err != nil {
		return err
	}
	record.Set("token", fpRecord.Token)
	record.Set("fingerprint", fingerprint)
	return h.SaveNoValidate(record)
}

// getRealIP extracts the client's real IP address from request headers,
// checking common proxy headers before falling back to the remote address.
func getRealIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// X-Forwarded-For can contain a comma-separated list: "client_ip, proxy1, proxy2"
		// Take the first one
		ips := strings.Split(ip, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
