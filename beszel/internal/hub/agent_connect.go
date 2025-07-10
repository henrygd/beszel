package hub

import (
	"beszel/internal/common"
	"beszel/internal/hub/expirymap"
	"beszel/internal/hub/ws"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/lxzan/gws"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// tokenMap maps tokens to user IDs for universal tokens
var tokenMap *expirymap.ExpiryMap[string]

type agentConnectRequest struct {
	token       string
	agentSemVer semver.Version
	// for universal token
	isUniversalToken bool
	userId           string
	remoteAddr       string
}

// validateAgentHeaders validates the required headers from agent connection requests.
func (h *Hub) validateAgentHeaders(headers http.Header) (string, string, error) {
	token := headers.Get("X-Token")
	agentVersion := headers.Get("X-Beszel")

	if agentVersion == "" || token == "" || len(token) > 512 {
		return "", "", errors.New("")
	}
	return token, agentVersion, nil
}

// getFingerprintRecord retrieves fingerprint data from the database by token.
func (h *Hub) getFingerprintRecord(token string, recordData *ws.FingerprintRecord) error {
	err := h.DB().NewQuery("SELECT id, system, fingerprint, token FROM fingerprints WHERE token = {:token}").
		Bind(dbx.Params{
			"token": token,
		}).
		One(recordData)
	return err
}

// sendResponseError sends an HTTP error response with the given status code and message.
func sendResponseError(res http.ResponseWriter, code int, message string) error {
	res.WriteHeader(code)
	if message != "" {
		res.Write([]byte(message))
	}
	return nil
}

// handleAgentConnect handles the incoming connection request from the agent.
func (h *Hub) handleAgentConnect(e *core.RequestEvent) error {
	if err := h.agentConnect(e.Request, e.Response); err != nil {
		return err
	}
	return nil
}

// agentConnect handles agent connection requests, validating credentials and upgrading to WebSocket.
func (h *Hub) agentConnect(req *http.Request, res http.ResponseWriter) (err error) {
	var agentConnectRequest agentConnectRequest
	var agentVersion string
	// check if user agent and token are valid
	agentConnectRequest.token, agentVersion, err = h.validateAgentHeaders(req.Header)
	if err != nil {
		return sendResponseError(res, http.StatusUnauthorized, "")
	}

	// Pull fingerprint from database matching token
	var fpRecord ws.FingerprintRecord
	err = h.getFingerprintRecord(agentConnectRequest.token, &fpRecord)

	// if no existing record, check if token is a universal token
	if err != nil {
		if err = checkUniversalToken(&agentConnectRequest); err == nil {
			// if this is a universal token, set the remote address and new record token
			agentConnectRequest.remoteAddr = getRealIP(req)
			fpRecord.Token = agentConnectRequest.token
		}
	}

	// If no matching token, return unauthorized
	if err != nil {
		return sendResponseError(res, http.StatusUnauthorized, "Invalid token")
	}

	// Validate agent version
	agentConnectRequest.agentSemVer, err = semver.Parse(agentVersion)
	if err != nil {
		return sendResponseError(res, http.StatusUnauthorized, "Invalid agent version")
	}

	// Upgrade connection to WebSocket
	conn, err := ws.GetUpgrader().Upgrade(res, req)
	if err != nil {
		return sendResponseError(res, http.StatusInternalServerError, "WebSocket upgrade failed")
	}

	go h.verifyWsConn(conn, agentConnectRequest, fpRecord)

	return nil
}

// verifyWsConn verifies the WebSocket connection using agent's fingerprint and SSH key signature.
func (h *Hub) verifyWsConn(conn *gws.Conn, acr agentConnectRequest, fpRecord ws.FingerprintRecord) (err error) {
	wsConn := ws.NewWsConnection(conn)
	// must be set before the read loop
	conn.Session().Store("wsConn", wsConn)

	// make sure connection is closed if there is an error
	defer func() {
		if err != nil {
			wsConn.Close()
			h.Logger().Error("WebSocket error", "error", err, "system", fpRecord.SystemId)
		}
	}()

	go conn.ReadLoop()

	signer, err := h.GetSSHKey("")
	if err != nil {
		return err
	}

	agentFingerprint, err := wsConn.GetFingerprint(acr.token, signer, acr.isUniversalToken)
	if err != nil {
		return err
	}

	// Create system if using universal token
	if acr.isUniversalToken {
		if acr.userId == "" {
			return errors.New("token user not found")
		}
		fpRecord.SystemId, err = h.createSystemFromAgentData(&acr, agentFingerprint)
		if err != nil {
			return fmt.Errorf("failed to create system from universal token: %w", err)
		}
	}

	switch {
	// If no current fingerprint, update with new fingerprint (first time connecting)
	case fpRecord.Fingerprint == "":
		if err := h.SetFingerprint(&fpRecord, agentFingerprint.Fingerprint); err != nil {
			return err
		}
	// Abort if fingerprint exists but doesn't match (different machine)
	case fpRecord.Fingerprint != agentFingerprint.Fingerprint:
		return errors.New("fingerprint mismatch")
	}

	return h.sm.AddWebSocketSystem(fpRecord.SystemId, acr.agentSemVer, wsConn)
}

// createSystemFromAgentData creates a new system record using data from the agent
func (h *Hub) createSystemFromAgentData(acr *agentConnectRequest, agentFingerprint common.FingerprintResponse) (recordId string, err error) {
	systemsCollection, err := h.FindCollectionByNameOrId("systems")
	if err != nil {
		return "", fmt.Errorf("failed to find systems collection: %w", err)
	}
	// separate port from address
	if agentFingerprint.Hostname == "" {
		agentFingerprint.Hostname = acr.remoteAddr
	}
	if agentFingerprint.Port == "" {
		agentFingerprint.Port = "45876"
	}
	// create new record
	systemRecord := core.NewRecord(systemsCollection)
	systemRecord.Set("name", agentFingerprint.Hostname)
	systemRecord.Set("host", acr.remoteAddr)
	systemRecord.Set("port", agentFingerprint.Port)
	systemRecord.Set("users", []string{acr.userId})

	return systemRecord.Id, h.Save(systemRecord)
}

// SetFingerprint updates the fingerprint for a given record ID.
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

func getTokenMap() *expirymap.ExpiryMap[string] {
	if tokenMap == nil {
		tokenMap = expirymap.New[string](time.Hour)
	}
	return tokenMap
}

func checkUniversalToken(acr *agentConnectRequest) (err error) {
	if tokenMap == nil {
		tokenMap = expirymap.New[string](time.Hour)
	}
	acr.userId, acr.isUniversalToken = tokenMap.GetOk(acr.token)
	if !acr.isUniversalToken {
		return errors.New("invalid token")
	}
	return nil
}

// getRealIP attempts to extract the real IP address from the request headers.
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
