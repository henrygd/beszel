package hub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"uuid"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
	"github.com/blang/semver"
	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/alerts"
	"github.com/henrygd/beszel/internal/ghupdate"
	"github.com/henrygd/beszel/internal/hub/config"
	"github.com/henrygd/beszel/internal/hub/heartbeat"
	"github.com/henrygd/beszel/internal/hub/mcp"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/hub/utils"
)

// UpdateInfo holds information about the latest update check
type UpdateInfo struct {
	lastCheck time.Time
	Version   string `json:"v"`
	Url       string `json:"url"`
}

var containerIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)

// Middleware to allow only admin role users
var requireAdminRole = customAuthMiddleware(func(e *core.RequestEvent) bool {
	return e.Auth.GetString("role") == "admin"
})

// Middleware to exclude readonly users
var excludeReadOnlyRole = customAuthMiddleware(func(e *core.RequestEvent) bool {
	return e.Auth.GetString("role") != "readonly"
})

// customAuthMiddleware handles boilerplate for custom authentication middlewares. fn should
// return true if the request is allowed, false otherwise. e.Auth is guaranteed to be non-nil.
func customAuthMiddleware(fn func(*core.RequestEvent) bool) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.UnauthorizedError("The request requires valid record authorization token.", nil)
		}
		if !fn(e) {
			return e.ForbiddenError("The authorized record is not allowed to perform this action.", nil)
		}
		return e.Next()
	}
}

// registerMiddlewares registers custom middlewares
func (h *Hub) registerMiddlewares(se *core.ServeEvent) {
	// authorizes request with user matching the provided email
	authorizeRequestWithEmail := func(e *core.RequestEvent, email string) (err error) {
		if e.Auth != nil || email == "" {
			return e.Next()
		}
		isAuthRefresh := e.Request.URL.Path == "/api/collections/users/auth-refresh" && e.Request.Method == http.MethodPost
		e.Auth, err = e.App.FindAuthRecordByEmail("users", email)
		// log on the auth refresh only: the login page sends one per attempt,
		// while this middleware runs on every request
		if isAuthRefresh && errors.Is(err, sql.ErrNoRows) {
			slog.Warn("No user matches the automatic login email, showing the login form", "email", email)
		}
		if err != nil || !isAuthRefresh {
			return e.Next()
		}
		// auth refresh endpoint, make sure token is set in header
		token, _ := e.Auth.NewAuthToken()
		e.Request.Header.Set("Authorization", token)
		return e.Next()
	}
	// authenticate with trusted header
	if autoLogin, _ := utils.GetEnv("AUTO_LOGIN"); autoLogin != "" {
		se.Router.BindFunc(func(e *core.RequestEvent) error {
			return authorizeRequestWithEmail(e, autoLogin)
		})
	}
	// authenticate with trusted header
	if trustedHeader, _ := utils.GetEnv("TRUSTED_AUTH_HEADER"); trustedHeader != "" {
		// only honor the header from these peers, if set
		trustedProxies, restricted := parseTrustedProxies()
		if !restricted {
			slog.Warn("TRUSTED_AUTH_HEADER is set without TRUSTED_PROXY_IPS: the header is trusted from any client that can reach the hub", "header", trustedHeader)
		}
		se.Router.BindFunc(func(e *core.RequestEvent) error {
			if restricted && !isTrustedProxy(trustedProxies, e.Request.RemoteAddr) {
				return e.Next()
			}
			return authorizeRequestWithEmail(e, e.Request.Header.Get(trustedHeader))
		})
	}
}

// parseTrustedProxies reads TRUSTED_PROXY_IPS (comma-separated IPs or CIDRs).
// restricted is false when the variable is unset or empty, meaning the trusted
// header is accepted from any peer. Invalid entries are skipped with a warning,
// so a typo narrows the allowlist rather than widening it.
func parseTrustedProxies() (prefixes []netip.Prefix, restricted bool) {
	value, _ := utils.GetEnv("TRUSTED_PROXY_IPS")
	if value == "" {
		return nil, false
	}
	for entry := range strings.SplitSeq(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if prefix, err := parseProxyPrefix(entry); err == nil {
			prefixes = append(prefixes, prefix)
		} else {
			slog.Warn("Ignoring invalid TRUSTED_PROXY_IPS entry", "entry", entry)
		}
	}
	return prefixes, true
}

// parseProxyPrefix parses an IP or CIDR into a masked prefix. IPv4-mapped IPv6
// entries are converted to IPv4 so they match IPv4 peers.
func parseProxyPrefix(entry string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(entry)
	if err != nil {
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return netip.Prefix{}, err
		}
		addr = addr.Unmap()
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	if prefix.Addr().Is4In6() {
		if prefix.Bits() < 96 {
			return netip.Prefix{}, fmt.Errorf("%s covers more than the IPv4-mapped range", entry)
		}
		prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
	}
	return prefix.Masked(), nil
}

// isTrustedProxy reports whether the peer address of a request (host:port) is
// within one of the prefixes.
func isTrustedProxy(prefixes []netip.Prefix, remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap().WithZone("")
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// registerApiRoutes registers custom API routes
func (h *Hub) registerApiRoutes(se *core.ServeEvent) error {
	// auth protected routes
	apiAuth := se.Router.Group("/api/beszel")
	apiAuth.Bind(apis.RequireAuth())
	// auth optional routes
	apiNoAuth := se.Router.Group("/api/beszel")

	// create first user endpoint only needed if no users exist
	if totalUsers, _ := se.App.CountRecords("users"); totalUsers == 0 {
		apiNoAuth.POST("/create-user", h.um.CreateFirstUser)
	}
	// check if first time setup on login page
	apiNoAuth.GET("/first-run", func(e *core.RequestEvent) error {
		total, err := e.App.CountRecords("users")
		return e.JSON(http.StatusOK, map[string]bool{"firstRun": err == nil && total == 0})
	})
	// get public key and version
	apiAuth.GET("/info", h.getInfo)
	apiAuth.GET("/getkey", h.getInfo) // deprecated - keep for compatibility w/ integrations
	// check for updates
	if optIn, _ := utils.GetEnv("CHECK_UPDATES"); optIn == "true" {
		var updateInfo UpdateInfo
		apiAuth.GET("/update", updateInfo.getUpdate)
	}
	// send test notification
	apiAuth.POST("/test-notification", h.SendTestNotification)
	// heartbeat status and test
	apiAuth.GET("/heartbeat-status", h.getHeartbeatStatus).BindFunc(requireAdminRole)
	apiAuth.POST("/heartbeat-status", h.updateHeartbeatStatus).BindFunc(requireAdminRole)
	apiAuth.POST("/test-heartbeat", h.testHeartbeat).BindFunc(requireAdminRole)
	// get config.yml content
	apiAuth.GET("/config-yaml", config.GetYamlConfig).BindFunc(requireAdminRole)
	// handle agent websocket connection
	apiNoAuth.GET("/agent-connect", h.handleAgentConnect)
	apiNoAuth.POST("/recovery/ping", h.handleRecoveryPing)
	// get, create, or delete universal tokens (POST keeps tokens out of URLs; GET is kept for legacy clients)
	apiAuth.GET("/universal-token", h.handleUniversalToken).BindFunc(excludeReadOnlyRole)
	apiAuth.POST("/universal-token", h.handleUniversalToken).BindFunc(excludeReadOnlyRole)
	// create static API keys for the current user, or revoke all of them
	apiAuth.POST("/api-keys", createApiKey).BindFunc(requireSessionToken)
	apiAuth.POST("/api-keys/revoke", revokeApiKeys).BindFunc(requireSessionToken)
	// update / delete user alerts
	apiAuth.POST("/user-alerts", alerts.UpsertUserAlerts)
	apiAuth.DELETE("/user-alerts", alerts.DeleteUserAlerts)
	// refresh SMART devices for a system
	apiAuth.POST("/smart/refresh", h.refreshSmartData).BindFunc(excludeReadOnlyRole)
	// refresh ZFS pool details for a system
	apiAuth.POST("/zfs/refresh", h.refreshZfsData).BindFunc(excludeReadOnlyRole)
	// get systemd service details
	apiAuth.GET("/systemd/info", h.getSystemdInfo)
	// get recent logs for a systemd service
	apiAuth.GET("/systemd/logs", h.getSystemdLogs)
	// get pending package updates
	apiAuth.GET("/package-updates", h.getPackageUpdates)
	// /containers routes
	if enabled, _ := utils.GetEnv("CONTAINER_DETAILS"); enabled != "false" {
		// get container logs
		apiAuth.GET("/containers/logs", h.getContainerLogs)
		// get container info
		apiAuth.GET("/containers/info", h.getContainerInfo)
	}
	// recovery routes
	apiAuth.GET("/recovery/modules", h.getRecoveryModules)
	apiAuth.GET("/recovery/module", h.getRecoveryModule)
	apiAuth.GET("/recovery/events", h.getRecoveryEvents)
	apiAuth.POST("/recovery/wake", h.triggerManualWOL)
	apiAuth.POST("/recovery/relay", h.triggerManualRelay)
	apiAuth.POST("/recovery/shutdown", h.triggerManualShutdown)
	apiAuth.POST("/recovery/force-restart", h.triggerManualForceRestart)
	apiAuth.GET("/recovery/module/conflict", h.getRecoveryModuleConflict)
	apiAuth.POST("/recovery/module/conflict", h.resolveRecoveryModuleConflict)
	apiAuth.GET("/recovery/stats", h.getRecoveryStats)
	// model context protocol (MCP) server for AI agents, enabled with MCP=true
	if err := mcp.RegisterRoutes(h, se); err != nil {
		return err
	}
	return nil
}

// getInfo returns data needed by authenticated users, such as the public key and current version
func (h *Hub) getInfo(e *core.RequestEvent) error {
	type infoResponse struct {
		Key         string `json:"key"`
		Version     string `json:"v"`
		CheckUpdate bool   `json:"cu"`
	}
	h.sshKeyMu.Lock()
	pubKey := h.pubKey
	h.sshKeyMu.Unlock()
	info := infoResponse{
		Key:     pubKey,
		Version: beszel.Version,
	}
	if optIn, _ := utils.GetEnv("CHECK_UPDATES"); optIn == "true" {
		info.CheckUpdate = true
	}
	return e.JSON(http.StatusOK, info)
}

// getUpdate checks for the latest release on GitHub and returns update info if a newer version is available
func (info *UpdateInfo) getUpdate(e *core.RequestEvent) error {
	if time.Since(info.lastCheck) < 6*time.Hour {
		return e.JSON(http.StatusOK, info)
	}
	info.lastCheck = time.Now()
	latestRelease, err := ghupdate.FetchLatestRelease(context.Background(), http.DefaultClient, "")
	if err != nil {
		return err
	}
	currentVersion, err := semver.Parse(strings.TrimPrefix(beszel.Version, "v"))
	if err != nil {
		return err
	}
	latestVersion, err := semver.Parse(strings.TrimPrefix(latestRelease.Tag, "v"))
	if err != nil {
		return err
	}
	if latestVersion.GT(currentVersion) {
		info.Version = strings.TrimPrefix(latestRelease.Tag, "v")
		info.Url = latestRelease.Url
	}
	return e.JSON(http.StatusOK, info)
}

// universalTokenRequest holds the options for the universal token endpoint.
// Enable and Permanent use 0 for false, 1 for true, and -1 for unset.
type universalTokenRequest struct {
	Token     string `json:"token"`
	Enable    int    `json:"enable"`
	Permanent int    `json:"permanent"`
}

// handleUniversalToken handles the universal token API endpoint (create, read, delete)
func (h *Hub) handleUniversalToken(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	if e.Auth.IsSuperuser() {
		return e.ForbiddenError("Superusers cannot use universal tokens", nil)
	}

	req := universalTokenRequest{Enable: -1, Permanent: -1}
	if e.Request.Method == http.MethodPost {
		if err := e.BindBody(&req); err != nil {
			return e.BadRequestError("Invalid universal token request", err)
		}
	} else {
		// legacy GET requests pass options as query params
		query := e.Request.URL.Query()
		req.Token = query.Get("token")
		if v, err := strconv.Atoi(query.Get("enable")); err == nil {
			req.Enable = v
		}
		if v, err := strconv.Atoi(query.Get("permanent")); err == nil {
			req.Permanent = v
		}
	}

	tokenMap := universalTokenMap.GetMap()
	userID := e.Auth.Id
	token := req.Token

	// helper for deleting any existing permanent token record for this user
	deletePermanent := func() error {
		rec, err := h.FindFirstRecordByFilter("universal_tokens", "user = {:user}", dbx.Params{"user": userID})
		if err != nil {
			return nil // no record
		}
		return h.Delete(rec)
	}

	// helper for upserting a permanent token record for this user
	upsertPermanent := func(token string) error {
		rec, err := h.FindFirstRecordByFilter("universal_tokens", "user = {:user}", dbx.Params{"user": userID})
		if err == nil {
			rec.Set("token", token)
			return h.Save(rec)
		}

		col, err := h.FindCachedCollectionByNameOrId("universal_tokens")
		if err != nil {
			return err
		}
		newRec := core.NewRecord(col)
		newRec.Set("user", userID)
		newRec.Set("token", token)
		return h.Save(newRec)
	}

	// Disable universal tokens (both ephemeral and permanent)
	if req.Enable == 0 {
		tokenMap.RemovebyValue(userID)
		_ = deletePermanent()
		return e.JSON(http.StatusOK, map[string]any{"token": token, "active": false, "permanent": false})
	}

	// Enable universal token (ephemeral or permanent)
	if req.Enable == 1 {
		if token == "" {
			token = uuid.New().String()
		}

		if req.Permanent == 1 {
			// make token permanent (persist across restarts)
			tokenMap.RemovebyValue(userID)
			if err := upsertPermanent(token); err != nil {
				return err
			}
			return e.JSON(http.StatusOK, map[string]any{"token": token, "active": true, "permanent": true})
		}

		// default: ephemeral mode (1 hour)
		_ = deletePermanent()
		tokenMap.Set(token, userID, time.Hour)
		return e.JSON(http.StatusOK, map[string]any{"token": token, "active": true, "permanent": false})
	}

	// Read current state
	// Prefer permanent token if it exists.
	if rec, err := h.FindFirstRecordByFilter("universal_tokens", "user = {:user}", dbx.Params{"user": userID}); err == nil {
		dbToken := rec.GetString("token")
		// If no token was provided, or the caller is asking about their permanent token, return it.
		if token == "" || token == dbToken {
			return e.JSON(http.StatusOK, map[string]any{"token": dbToken, "active": true, "permanent": true})
		}
		// Token doesn't match their permanent token (avoid leaking other info)
		return e.JSON(http.StatusOK, map[string]any{"token": token, "active": false, "permanent": false})
	}

	// No permanent token; fall back to ephemeral token map.
	if token == "" {
		// return existing token if it exists
		if token, _, ok := tokenMap.GetByValue(userID); ok {
			return e.JSON(http.StatusOK, map[string]any{"token": token, "active": true, "permanent": false})
		}
		// if no token is provided, generate a new one
		token = uuid.New().String()
	}

	// Token is considered active only if it belongs to the current user.
	activeUser, ok := tokenMap.GetOk(token)
	active := ok && activeUser == userID
	response := map[string]any{"token": token, "active": active, "permanent": false}
	return e.JSON(http.StatusOK, response)
}

// getHeartbeatStatus returns current heartbeat configuration and whether it's enabled
func (h *Hub) getHeartbeatStatus(e *core.RequestEvent) error {
	h.hbMu.RLock()
	defer h.hbMu.RUnlock()

	enabled, cfg := heartbeat.GetConfigFromFileOrEnv(e.App, utils.GetEnv)
	return e.JSON(http.StatusOK, map[string]any{
		"enabled":  enabled,
		"url":      cfg.URL,
		"interval": cfg.Interval,
		"method":   cfg.Method,
	})
}

// updateHeartbeatStatus saves new heartbeat settings and restarts the heartbeat ticker
func (h *Hub) updateHeartbeatStatus(e *core.RequestEvent) error {
	var req struct {
		URL      string `json:"url"`
		Interval int    `json:"interval"`
		Method   string `json:"method"`
	}
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Invalid request body", err)
	}

	req.URL = strings.TrimSpace(req.URL)
	if req.URL != "" {
		if _, err := url.Parse(req.URL); err != nil {
			return e.BadRequestError("Invalid URL", err)
		}
		if req.Interval <= 0 {
			req.Interval = 60
		}
		req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
		if req.Method != http.MethodGet && req.Method != http.MethodHead && req.Method != http.MethodPost {
			req.Method = http.MethodPost
		}
	}

	// Save to JSON config file
	if err := heartbeat.SaveConfig(e.App, req.URL, req.Interval, req.Method); err != nil {
		return e.InternalServerError("Failed to save settings", err)
	}

	// Stop old heartbeat if running
	h.hbMu.Lock()
	defer h.hbMu.Unlock()

	if h.hbStop != nil {
		close(h.hbStop)
		h.hbStop = nil
	}
	h.hb = nil

	// Start new heartbeat if URL is provided
	if req.URL != "" {
		h.hb = heartbeat.New(e.App, utils.GetEnv)
		if h.hb != nil {
			h.hbStop = make(chan struct{})
			go h.hb.Start(h.hbStop)
		}
	}

	return e.JSON(http.StatusOK, map[string]any{"success": true})
}

// testHeartbeat triggers a single heartbeat ping and returns the result
func (h *Hub) testHeartbeat(e *core.RequestEvent) error {
	h.hbMu.RLock()
	hb := h.hb
	h.hbMu.RUnlock()

	if hb == nil {
		return e.JSON(http.StatusOK, map[string]any{
			"err": "Heartbeat not configured.",
		})
	}
	if err := hb.Send(); err != nil {
		return e.JSON(http.StatusOK, map[string]any{"err": err.Error()})
	}
	return e.JSON(http.StatusOK, map[string]any{"err": false})
}

// containerRequestHandler handles both container logs and info requests
func (h *Hub) containerRequestHandler(e *core.RequestEvent, fetchFunc func(*systems.System, string) (string, error), responseKey string) error {
	systemID := e.Request.URL.Query().Get("system")
	containerID := e.Request.URL.Query().Get("container")

	if systemID == "" || containerID == "" || !containerIDPattern.MatchString(containerID) {
		return e.BadRequestError("Invalid system or container parameter", nil)
	}

	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}

	data, err := fetchFunc(system, containerID)
	if err != nil {
		return e.InternalServerError("", err)
	}

	return e.JSON(http.StatusOK, map[string]string{responseKey: data})
}

// getContainerLogs handles GET /api/beszel/containers/logs requests
func (h *Hub) getContainerLogs(e *core.RequestEvent) error {
	return h.containerRequestHandler(e, func(system *systems.System, containerID string) (string, error) {
		return system.FetchContainerLogsFromAgent(containerID)
	}, "logs")
}

func (h *Hub) getContainerInfo(e *core.RequestEvent) error {
	return h.containerRequestHandler(e, func(system *systems.System, containerID string) (string, error) {
		return system.FetchContainerInfoFromAgent(containerID)
	}, "info")
}

// getSystemdInfo handles GET /api/beszel/systemd/info requests
func (h *Hub) getSystemdInfo(e *core.RequestEvent) error {
	query := e.Request.URL.Query()
	systemID := query.Get("system")
	serviceName := query.Get("service")

	if systemID == "" || serviceName == "" {
		return e.BadRequestError("Invalid system or service parameter", nil)
	}
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}
	// verify service exists before fetching details
	_, err = e.App.FindFirstRecordByFilter("systemd_services", "system = {:system} && name = {:name}", dbx.Params{
		"system": systemID,
		"name":   serviceName,
	})
	if err != nil {
		return e.NotFoundError("", err)
	}
	details, err := system.FetchSystemdInfoFromAgent(serviceName)
	if err != nil {
		return e.InternalServerError("", err)
	}
	e.Response.Header().Set("Cache-Control", "private, max-age=60")
	e.Response.Header().Add("Vary", "Authorization")
	return e.JSON(http.StatusOK, map[string]any{"details": details})
}

// getSystemdLogs handles GET /api/beszel/systemd/logs requests.
func (h *Hub) getSystemdLogs(e *core.RequestEvent) error {
	query := e.Request.URL.Query()
	systemID := query.Get("system")
	serviceName := query.Get("service")

	if systemID == "" || serviceName == "" {
		return e.BadRequestError("Invalid system or service parameter", nil)
	}
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}
	// Only fetch logs for services that are currently monitored on this system.
	_, err = e.App.FindFirstRecordByFilter("systemd_services", "system = {:system} && name = {:name}", dbx.Params{
		"system": systemID,
		"name":   serviceName,
	})
	if err != nil {
		return e.NotFoundError("", err)
	}
	// Old agents and agents without journal access do not advertise this capability.
	systemRecord, err := e.App.FindRecordById("systems", systemID)
	if err != nil {
		return e.NotFoundError("", err)
	}
	var info struct {
		SystemdLogs bool `json:"jl"`
	}
	if err := systemRecord.UnmarshalJSONField("info", &info); err != nil {
		e.App.Logger().Error("failed to unmarshal system info for systemd logs", "system", systemID, "err", err)
		return e.JSON(http.StatusOK, map[string]string{"logs": ""})
	}
	if !info.SystemdLogs {
		return e.JSON(http.StatusOK, map[string]string{"logs": ""})
	}

	logs, err := system.FetchSystemdLogsFromAgent(serviceName)
	if err != nil {
		return e.InternalServerError("", err)
	}
	return e.JSON(http.StatusOK, map[string]string{"logs": logs})
}

// getPackageUpdates handles GET /api/beszel/package-updates requests
func (h *Hub) getPackageUpdates(e *core.RequestEvent) error {
	systemID := e.Request.URL.Query().Get("system")
	if systemID == "" {
		return e.BadRequestError("Invalid system parameter", nil)
	}
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}
	updates, err := system.FetchPackageUpdatesFromAgent()
	if err != nil {
		return e.InternalServerError("", err)
	}
	return e.JSON(http.StatusOK, updates)
}

// refreshSmartData handles POST /api/beszel/smart/refresh requests
// Fetches fresh SMART data from the agent and updates the collection
func (h *Hub) refreshSmartData(e *core.RequestEvent) error {
	systemID := e.Request.URL.Query().Get("system")
	if systemID == "" {
		return e.BadRequestError("Invalid system parameter", nil)
	}

	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}

	if err := system.FetchAndSaveSmartDevices(); err != nil {
		return e.InternalServerError("", err)
	}

	return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// isRecoveryModuleOnline reports real module liveness from the dedicated
// last_ping heartbeat timestamp: online when the module pinged within
// max(90s, 3x its configured ping interval). The `status` string field only
// tracks approval state (unapproved/online/disabled/...) and the `updated`
// autodate is bumped by any UI edit, so neither is a trustworthy liveness
// signal. Records created before the last_ping migration fall back to
// `updated` until their first post-upgrade ping.
func isRecoveryModuleOnline(rec *core.Record) bool {
	lastPing := rec.GetDateTime("last_ping")
	if lastPing.IsZero() {
		lastPing = rec.GetDateTime("updated")
		if lastPing.IsZero() {
			return false
		}
	}
	staleAfter := 90 * time.Second
	if interval := rec.GetInt("ping_interval_seconds"); interval > 0 {
		if threshold := 3 * time.Duration(interval) * time.Second; threshold > staleAfter {
			staleAfter = threshold
		}
	}
	return time.Since(lastPing.Time()) < staleAfter
}

// computeRecoverySyncStatus derives the module's config-sync state from
// desired (config_revision/config_hash) vs. reported
// (reported_config_revision/reported_config_hash) values, plus whether an
// ESP-side change is pending manual conflict resolution. Revision numbers
// are the authoritative signal; hash is only used to catch a same-revision
// mismatch (SYNC_ERROR) since the ESP and hub don't share a hash algorithm.
func computeRecoverySyncStatus(rec *core.Record) string {
	if rec.GetBool("pending_esp_change") {
		return "CONFLICT"
	}
	desiredRev := rec.GetInt("config_revision")
	reportedRev := rec.GetInt("reported_config_revision")

	if !isRecoveryModuleOnline(rec) {
		if reportedRev < desiredRev {
			return "OFFLINE_PENDING"
		}
		return "SYNCED"
	}
	if reportedRev == desiredRev {
		desiredHash := rec.GetString("config_hash")
		reportedHash := rec.GetString("reported_config_hash")
		if desiredHash != "" && reportedHash != "" && desiredHash != reportedHash {
			return "SYNC_ERROR"
		}
		return "SYNCED"
	}
	if reportedRev < desiredRev {
		return "SYNC_PENDING"
	}
	// reportedRev > desiredRev without pending_esp_change - the ESP is ahead
	// of what the hub expects outside of the normal accept/conflict path.
	return "SYNC_ERROR"
}

// recoveryHealthScore is the weighted 0-100 Recovery Protection score plus a
// breakdown of any lost points, adapted to telemetry Beszel actually
// collects today (no reset-reason counting yet, so "firmware/integration"
// stands in for reset stability).
type recoveryHealthScore struct {
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

// computeRecoveryHealthScore weighs: heartbeat/online (30), config sync (20),
// channel protection (20), gateway health (10), temperature (10), and
// firmware/integration (10).
func computeRecoveryHealthScore(rec *core.Record, syncStatus string, channels []map[string]any) recoveryHealthScore {
	score := 0
	var reasons []string

	if rec.GetString("status") == "unapproved" {
		reasons = append(reasons, "Module is awaiting approval")
	} else if isRecoveryModuleOnline(rec) {
		score += 30
	} else {
		reasons = append(reasons, "Module is offline")
	}

	if syncStatus == "SYNCED" {
		score += 20
	} else {
		reasons = append(reasons, fmt.Sprintf("Configuration sync status is %s", syncStatus))
	}

	if total := len(channels); total > 0 {
		protected := 0
		for _, ch := range channels {
			hasSystem, _ := ch["system"].(string)
			disabled, _ := ch["hardware_recovery_disabled"].(bool)
			if hasSystem != "" && !disabled {
				protected++
			}
		}
		score += int(20 * float64(protected) / float64(total))
		if protected < total {
			reasons = append(reasons, fmt.Sprintf("%d of %d channels lack full protection", total-protected, total))
		}
	} else {
		reasons = append(reasons, "No channels configured")
	}

	if rec.GetString("gateway_ip") == "" {
		score += 10 // no gateway configured to check - don't penalize
	} else if rec.GetBool("gateway_online") {
		score += 10
	} else {
		reasons = append(reasons, "Gateway is unreachable")
	}

	if rec.GetBool("temperature_monitoring_disabled") {
		score += 10 // intentionally disabled - don't penalize
	} else {
		temp := rec.GetFloat("temperature")
		warn := rec.GetFloat("temp_threshold_warning")
		crit := rec.GetFloat("temp_threshold_critical")
		if warn == 0 {
			warn = 50
		}
		if crit == 0 {
			crit = 60
		}
		if temp >= crit {
			reasons = append(reasons, fmt.Sprintf("Temperature critical (%.1f°C)", temp))
		} else if temp >= warn {
			score += 5
			reasons = append(reasons, fmt.Sprintf("Temperature elevated (%.1f°C)", temp))
		} else {
			score += 10
		}
	}

	if rec.GetString("firmware_version") != "" && rec.GetString("status") != "unapproved" {
		score += 10
	} else {
		reasons = append(reasons, "Module not fully integrated")
	}

	return recoveryHealthScore{Score: score, Reasons: reasons}
}

// buildRecoveryModuleResponse assembles the full JSON response for a single
// recovery_modules record, shared by getRecoveryModules and getRecoveryModule
// so both endpoints stay in sync as fields are added.
func (h *Hub) buildRecoveryModuleResponse(e *core.RequestEvent, rec *core.Record) map[string]any {
	var channels []map[string]any
	chRecords, _ := e.App.FindRecordsByFilter("recovery_channels", "module = {:module}", "", -1, 0, dbx.Params{"module": rec.Id})
	for _, ch := range chRecords {
		channels = append(channels, map[string]any{
			"system":                     ch.GetString("system"),
			"hardware_recovery_disabled": ch.GetBool("hardware_recovery_disabled"),
		})
	}

	syncStatus := computeRecoverySyncStatus(rec)
	health := computeRecoveryHealthScore(rec, syncStatus, channels)

	return map[string]any{
		"id":                               rec.Id,
		"name":                             rec.GetString("name"),
		"mac_address":                      rec.GetString("mac_address"),
		"ip_address":                       rec.GetString("ip_address"),
		"online":                           isRecoveryModuleOnline(rec),
		"last_ping":                        rec.GetDateTime("last_ping"),
		"gateway_ip":                       rec.GetString("gateway_ip"),
		"gateway_name":                     rec.GetString("gateway_name"),
		"gateway_online":                   rec.GetBool("gateway_online"),
		"max_channels":                     rec.GetInt("max_channels"),
		"firmware_version":                 rec.GetString("firmware_version"),
		"status":                           rec.GetString("status"),
		"config_revision":                  rec.GetInt("config_revision"),
		"config_hash":                      rec.GetString("config_hash"),
		"reported_config_revision":         rec.GetInt("reported_config_revision"),
		"reported_config_hash":             rec.GetString("reported_config_hash"),
		"last_config_source":               rec.GetString("last_config_source"),
		"pending_esp_change":               rec.GetBool("pending_esp_change"),
		"sync_status":                      syncStatus,
		"health_score":                     health.Score,
		"health_reasons":                   health.Reasons,
		"ping_interval_seconds":            rec.GetInt("ping_interval_seconds"),
		"temperature":                      rec.GetFloat("temperature"),
		"temperature_monitoring_disabled":  rec.GetBool("temperature_monitoring_disabled"),
		"temp_threshold_warning":           rec.GetFloat("temp_threshold_warning"),
		"temp_threshold_critical":          rec.GetFloat("temp_threshold_critical"),
		"buzzer_disabled":                  rec.GetBool("buzzer_disabled"),
		"buzzer_muted":                     rec.GetBool("buzzer_muted"),
		"created":                          rec.GetDateTime("created").Time(),
		"updated":                          rec.GetDateTime("updated").Time(),
	}
}

// getRecoveryModules handles GET /api/beszel/recovery/modules requests
func (h *Hub) getRecoveryModules(e *core.RequestEvent) error {
	var modules []map[string]any
	records, err := e.App.FindRecordsByFilter("recovery_modules", "", "-created", -1, 0)
	if err != nil {
		return e.InternalServerError("Failed to query recovery modules", err)
	}
	for _, rec := range records {
		modules = append(modules, h.buildRecoveryModuleResponse(e, rec))
	}
	return e.JSON(http.StatusOK, modules)
}

// getRecoveryModule handles GET /api/beszel/recovery/module requests
func (h *Hub) getRecoveryModule(e *core.RequestEvent) error {
	id := e.Request.URL.Query().Get("id")
	if id == "" {
		return e.BadRequestError("Missing module ID", nil)
	}
	rec, err := e.App.FindRecordById("recovery_modules", id)
	if err != nil {
		return e.NotFoundError("Recovery module not found", err)
	}
	return e.JSON(http.StatusOK, h.buildRecoveryModuleResponse(e, rec))
}

// getRecoveryModuleConflict handles GET /api/beszel/recovery/module/conflict
// requests, returning the pending ESP-reported change alongside the current
// desired values so the frontend can render a side-by-side resolution.
func (h *Hub) getRecoveryModuleConflict(e *core.RequestEvent) error {
	id := e.Request.URL.Query().Get("id")
	if id == "" {
		return e.BadRequestError("Missing module ID", nil)
	}
	rec, err := e.App.FindRecordById("recovery_modules", id)
	if err != nil {
		return e.NotFoundError("Recovery module not found", err)
	}
	if !rec.GetBool("pending_esp_change") {
		return e.JSON(http.StatusOK, map[string]any{"pending": false})
	}
	var espChange recoveryLocalChangePayload
	_ = json.Unmarshal([]byte(rec.GetString("esp_change_payload")), &espChange)
	return e.JSON(http.StatusOK, map[string]any{
		"pending":    true,
		"esp_change": espChange,
		"desired": map[string]any{
			"config_revision": rec.GetInt("config_revision"),
			"config_hash":     rec.GetString("config_hash"),
		},
	})
}

// resolveRecoveryModuleConflict handles POST /api/beszel/recovery/module/conflict
// requests. use_esp=true applies the ESP's pending change as the new desired
// config; use_esp=false just keeps Beszel's current desired values and
// discards the ESP's proposal (the ESP will overwrite its own local change
// with the hub's desired config on its next ping).
func (h *Hub) resolveRecoveryModuleConflict(e *core.RequestEvent) error {
	var req struct {
		ModuleID string `json:"module_id"`
		UseEsp   bool   `json:"use_esp"`
	}
	if err := e.BindBody(&req); err != nil {
		return e.BadRequestError("Invalid request body", err)
	}
	rec, err := e.App.FindRecordById("recovery_modules", req.ModuleID)
	if err != nil {
		return e.NotFoundError("Recovery module not found", err)
	}
	if req.UseEsp {
		var espChange recoveryLocalChangePayload
		if err := json.Unmarshal([]byte(rec.GetString("esp_change_payload")), &espChange); err == nil {
			applyLocalModuleChange(rec, espChange.Module)
			if len(espChange.Channels) > 0 {
				applyLocalChannelChanges(e.App, rec.Id, espChange.Channels)
			}
			rec.Set("config_revision", rec.GetInt("reported_config_revision"))
			rec.Set("config_hash", rec.GetString("reported_config_hash"))
			rec.Set("last_config_source", "ESP_WEB")
		}
	}
	rec.Set("pending_esp_change", false)
	rec.Set("esp_change_payload", nil)
	if err := e.App.Save(rec); err != nil {
		return e.InternalServerError("Failed to resolve conflict", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"status": "ok"})
}

// getRecoveryStats handles GET /api/beszel/recovery/stats requests, returning
// the recovery count and most recent recovery timestamp for a system.
func (h *Hub) getRecoveryStats(e *core.RequestEvent) error {
	systemID := e.Request.URL.Query().Get("system")
	if systemID == "" {
		return e.BadRequestError("Missing system ID", nil)
	}
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("System not found or access denied", nil)
	}
	var result struct {
		Count int    `db:"cnt"`
		Last  string `db:"last"`
	}
	err = e.App.DB().NewQuery(`
		SELECT COUNT(*) as cnt, COALESCE(MAX(timestamp), '') as last
		FROM recovery_events
		WHERE system = {:system} AND event IN ('WOL_SUCCESS', 'RELAY_SUCCESS', 'FAST_VERIFY_RECOVERED')
	`).Bind(dbx.Params{"system": systemID}).One(&result)
	if err != nil {
		return e.InternalServerError("Failed to query recovery stats", err)
	}
	resp := map[string]any{"recovery_count": result.Count, "last_recovery": nil}
	if result.Last != "" {
		if parsed, parseErr := time.Parse("2006-01-02 15:04:05.000Z", result.Last); parseErr == nil {
			resp["last_recovery"] = parsed
		} else if parsed, parseErr := time.Parse(time.RFC3339, result.Last); parseErr == nil {
			resp["last_recovery"] = parsed
		} else {
			resp["last_recovery"] = result.Last
		}
	}
	return e.JSON(http.StatusOK, resp)
}

// getRecoveryEvents handles GET /api/beszel/recovery/events requests
func (h *Hub) getRecoveryEvents(e *core.RequestEvent) error {
	systemID := e.Request.URL.Query().Get("system")
	if systemID == "" {
		return e.BadRequestError("Missing system ID", nil)
	}
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("System not found or access denied", nil)
	}
	records, err := e.App.FindRecordsByFilter("recovery_events", "system = {:system}", "-timestamp", -1, 0, dbx.Params{"system": systemID})
	if err != nil {
		return e.InternalServerError("Failed to query recovery events", err)
	}
	var events []map[string]any
	for _, rec := range records {
		events = append(events, map[string]any{
			"id":        rec.Id,
			"system":    rec.GetString("system"),
			"module":    rec.GetString("module"),
			"channel":   rec.GetInt("channel"),
			"event":     rec.GetString("event"),
			"timestamp": rec.GetDateTime("timestamp").Time(),
			"metadata":  rec.Get("metadata"),
		})
	}
	return e.JSON(http.StatusOK, events)
}

// triggerManualWOL handles POST /api/beszel/recovery/wake requests
func (h *Hub) triggerManualWOL(e *core.RequestEvent) error {
	systemID := e.Request.URL.Query().Get("system")
	if systemID == "" {
		return e.BadRequestError("Missing system ID", nil)
	}
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("System not found or access denied", nil)
	}
	rec, err := e.App.FindFirstRecordByFilter("recovery_channels", "system = {:system}", dbx.Params{"system": systemID})
	if err != nil {
		return e.BadRequestError("Wake-on-LAN is not configured for this system", err)
	}
	mac := rec.GetString("mac_address")
	bcast := rec.GetString("broadcast_address")
	port := rec.GetInt("wol_port")
	if mac == "" {
		return e.BadRequestError("Wake-on-LAN MAC address is not configured", nil)
	}
	if bcast == "" {
		bcast = "255.255.255.255"
	}
	if port <= 0 {
		port = 9
	}
	leaseID, acquired := h.sm.AcquireRecoveryLock(rec.Id, "MANUAL_UI", 15*time.Second)
	if !acquired {
		return e.JSON(http.StatusConflict, map[string]any{
			"status":  http.StatusConflict,
			"message": "A recovery action is already in progress for this system. Please wait for it to complete.",
			"data":    map[string]any{},
		})
	}
	defer h.sm.ReleaseRecoveryLock(rec.Id, leaseID)
	err = systems.SendMagicPacket(mac, bcast, port)
	if err != nil {
		return e.InternalServerError("Failed to broadcast Wake-on-LAN magic packet", err)
	}
	collection, err := e.App.FindCollectionByNameOrId("recovery_events")
	if err == nil {
		eventRec := core.NewRecord(collection)
		eventRec.Set("system", systemID)
		eventRec.Set("module", rec.GetString("module"))
		eventRec.Set("channel", rec.GetInt("channel_number"))
		eventRec.Set("event", "WOL_MANUAL_SENT")
		eventRec.Set("timestamp", time.Now().UTC())
		eventRec.Set("metadata", fmt.Sprintf(`{"mac":"%s","broadcast":"%s","port":%d,"source":"MANUAL_UI"}`, mac, bcast, port))
		_ = e.App.Save(eventRec)
	}
	return e.JSON(http.StatusOK, map[string]any{"status": "ok"})
}

type recoveryPingPayload struct {
	MACAddress      string   `json:"mac_address"`
	IPAddress       string   `json:"ip_address"`
	FirmwareVersion string   `json:"firmware_version"`
	MaxChannels     int      `json:"max_channels"`
	ConfigRevision  int      `json:"config_revision"` // what the ESP is currently running (reported state)
	ConfigHash      string   `json:"config_hash"`     // ditto
	Temperature     *float64 `json:"temperature,omitempty"`
	// LocalChange is present only when the ESP's local web portal just
	// applied a settings edit - it carries the changed fields plus the
	// desired-config revision the ESP based its edit on, so the hub can
	// tell whether to accept it or flag a conflict.
	LocalChange *recoveryLocalChangePayload `json:"local_change,omitempty"`
}

type recoveryLocalChangePayload struct {
	BaseRevision int              `json:"base_revision"`
	Module       map[string]any   `json:"module,omitempty"`
	Channels     []map[string]any `json:"channels,omitempty"`
}

// recoveryModuleFieldWhitelist lists the recovery_modules fields an ESP's
// local web portal is allowed to change. Anything not in this set is
// silently ignored, so a malformed/malicious local_change can't write
// arbitrary fields (e.g. status, mac_address).
var recoveryModuleFieldWhitelist = map[string]bool{
	"name":                            true,
	"gateway_ip":                      true,
	"gateway_name":                    true,
	"ping_interval_seconds":           true,
	"temperature_monitoring_disabled": true,
	"temp_threshold_warning":          true,
	"temp_threshold_critical":         true,
	"buzzer_disabled":                 true,
	"buzzer_muted":                    true,
}

// recoveryChannelFieldWhitelist lists the recovery_channels fields an ESP's
// local web portal is allowed to change (identified by "channel" number,
// not by record ID, since the ESP doesn't know PocketBase record IDs).
var recoveryChannelFieldWhitelist = map[string]bool{
	"host_ip":                    true,
	"probe_ports":                true,
	"failure_threshold":          true,
	"boot_grace_seconds":         true,
	"maintenance":                true,
	"hardware_recovery_disabled": true,
}

// applyLocalModuleChange writes only whitelisted fields from an ESP-reported
// local change onto a recovery_modules record.
func applyLocalModuleChange(rec *core.Record, fields map[string]any) {
	for k, v := range fields {
		if recoveryModuleFieldWhitelist[k] {
			rec.Set(k, v)
		}
	}
}

// toInt converts a JSON-decoded number (float64) or plain int to an int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

// applyLocalChannelChanges writes whitelisted fields from ESP-reported local
// channel edits onto the matching recovery_channels records, identified by
// channel number within the given module.
func applyLocalChannelChanges(app core.App, moduleID string, changes []map[string]any) {
	for _, change := range changes {
		chanNum, ok := toInt(change["channel"])
		if !ok {
			continue
		}
		chRec, err := app.FindFirstRecordByFilter("recovery_channels", "module = {:module} && channel_number = {:num}", dbx.Params{"module": moduleID, "num": chanNum})
		if err != nil {
			continue
		}
		for k, v := range change {
			if k != "channel" && recoveryChannelFieldWhitelist[k] {
				chRec.Set(k, v)
			}
		}
		_ = app.Save(chRec)
	}
}

// handleRecoveryPing handles POST /api/beszel/recovery/ping requests from ESP32 modules.
func (h *Hub) handleRecoveryPing(e *core.RequestEvent) error {
	var payload recoveryPingPayload
	if err := e.BindBody(&payload); err != nil {
		return e.BadRequestError("Invalid request payload", err)
	}
	if payload.MACAddress == "" || payload.IPAddress == "" {
		return e.BadRequestError("Missing MAC or IP Address", nil)
	}
	rec, err := e.App.FindFirstRecordByFilter("recovery_modules", "mac_address = {:mac}", dbx.Params{"mac": payload.MACAddress})
	collection, errCol := e.App.FindCollectionByNameOrId("recovery_modules")
	if errCol != nil {
		return e.InternalServerError("Recovery modules collection not found", errCol)
	}
	var isNew bool
	if err != nil {
		isNew = true
		rec = core.NewRecord(collection)
		rec.Set("mac_address", payload.MACAddress)
		rec.Set("name", fmt.Sprintf("Discovered ESP (%s)", payload.MACAddress))
		rec.Set("max_channels", payload.MaxChannels)
		rec.Set("status", "unapproved")
		// config_revision is a required field with no useful zero-value - a
		// first-time auto-discovered module needs a starting revision the
		// same way a manually-added one does (see handleAddModule/the "Add
		// Device" dialog, which also starts new modules at revision 1).
		rec.Set("config_revision", 1)
	}
	rec.Set("ip_address", payload.IPAddress)
	if payload.FirmwareVersion != "" {
		rec.Set("firmware_version", payload.FirmwareVersion)
	}
	if payload.Temperature != nil {
		temp := *payload.Temperature
		rec.Set("temperature", temp)
		warnThreshold := rec.GetFloat("temp_threshold_warning")
		critThreshold := rec.GetFloat("temp_threshold_critical")
		if warnThreshold == 0 {
			warnThreshold = 50
			rec.Set("temp_threshold_warning", 50)
		}
		if critThreshold == 0 {
			critThreshold = 60
			rec.Set("temp_threshold_critical", 60)
		}
		// Temperature is still recorded for display when monitoring is
		// disabled, but disabling it must stop alert decisions entirely.
		if rec.GetBool("temperature_monitoring_disabled") {
			// skip alerting
		} else if temp > critThreshold {
			admins, errAd := e.App.FindRecordsByFilter("users", "role = 'admin'", "", -1, 0)
			if errAd == nil {
				for _, admin := range admins {
					_ = h.AlertManager.SendAlert(alerts.AlertMessageData{
						UserID:   admin.Id,
						SystemID: "",
						Title:    fmt.Sprintf("[CRITICAL RACK TEMPERATURE] %s", rec.GetString("name")),
						Message:  fmt.Sprintf("Recovery Module %s ambient temperature is CRITICAL: %.1f°C (Threshold: %.1f°C)", rec.GetString("name"), temp, critThreshold),
						Link:     h.MakeLink("/settings/recovery"),
						LinkText: "Open Recovery Settings",
					})
				}
			}
		} else if temp > warnThreshold {
			admins, errAd := e.App.FindRecordsByFilter("users", "role = 'admin'", "", -1, 0)
			if errAd == nil {
				for _, admin := range admins {
					_ = h.AlertManager.SendAlert(alerts.AlertMessageData{
						UserID:   admin.Id,
						SystemID: "",
						Title:    fmt.Sprintf("[WARNING RACK TEMPERATURE] %s", rec.GetString("name")),
						Message:  fmt.Sprintf("Recovery Module %s ambient temperature is warning: %.1f°C (Threshold: %.1f°C)", rec.GetString("name"), temp, warnThreshold),
						Link:     h.MakeLink("/settings/recovery"),
						LinkText: "Open Recovery Settings",
					})
				}
			}
		}
	}

	// Always record what the ESP says it's currently running (reported
	// state), independent of whether it's also proposing a new local change.
	rec.Set("reported_config_revision", payload.ConfigRevision)
	rec.Set("reported_config_hash", payload.ConfigHash)

	if payload.LocalChange != nil && !isNew {
		if payload.LocalChange.BaseRevision == rec.GetInt("config_revision") {
			// No concurrent hub-side change since the ESP made this edit -
			// accept it as the new desired state.
			applyLocalModuleChange(rec, payload.LocalChange.Module)
			if len(payload.LocalChange.Channels) > 0 {
				applyLocalChannelChanges(e.App, rec.Id, payload.LocalChange.Channels)
			}
			rec.Set("config_revision", payload.ConfigRevision)
			rec.Set("config_hash", payload.ConfigHash)
			rec.Set("last_config_source", "ESP_WEB")
			rec.Set("pending_esp_change", false)
			rec.Set("esp_change_payload", nil)
		} else {
			// The hub's desired config already moved past what the ESP
			// based its edit on - don't blindly apply either side. Surface
			// both for an admin to resolve instead of last-write-wins.
			rec.Set("pending_esp_change", true)
			payloadJSON, _ := json.Marshal(payload.LocalChange)
			rec.Set("esp_change_payload", string(payloadJSON))
		}
	}

	// Dedicated heartbeat timestamp - the only writer of last_ping. The
	// autodate `updated` also bumps on UI edits, so liveness checks use this.
	rec.Set("last_ping", types.NowDateTime())
	// A previously stale module that pings again is immediately live; don't
	// wait for the offline scanner's next sweep to flip the status back.
	if rec.GetString("status") == "offline" {
		rec.Set("status", "online")
	}

	if errSave := e.App.Save(rec); errSave != nil {
		return e.InternalServerError("Failed to save module record", errSave)
	}
	if isNew {
		collectionEvents, errEv := e.App.FindCollectionByNameOrId("recovery_events")
		if errEv == nil {
			eventRec := core.NewRecord(collectionEvents)
			eventRec.Set("module", rec.Id)
			eventRec.Set("event", "MODULE_DISCOVERED")
			eventRec.Set("timestamp", time.Now().UTC())
			eventRec.Set("metadata", fmt.Sprintf(`{"mac":"%s","ip":"%s"}`, payload.MACAddress, payload.IPAddress))
			_ = e.App.Save(eventRec)
		}
	}
	if rec.GetString("status") == "unapproved" {
		return e.JSON(http.StatusOK, map[string]any{
			"status": "unapproved",
		})
	}
	var channels []map[string]any
	channelRecords, errChan := e.App.FindRecordsByFilter("recovery_channels", "module = {:module}", "", -1, 0, dbx.Params{"module": rec.Id})
	if errChan == nil {
		for _, chRec := range channelRecords {
			var ports []int
			portsData := chRec.GetString("probe_ports")
			if portsData != "" {
				_ = json.Unmarshal([]byte(portsData), &ports)
			}
			systemName := ""
			if systemID := chRec.GetString("system"); systemID != "" {
				if sysRec, errSys := e.App.FindRecordById("systems", systemID); errSys == nil {
					systemName = sysRec.GetString("name")
				}
			}
			channel := map[string]any{
				"channel":                    chRec.GetInt("channel_number"),
				"system":                     chRec.GetString("system"),
				"name":                       systemName,
				"host_ip":                    chRec.GetString("host_ip"),
				"ports":                      ports,
				"maintenance":                chRec.GetBool("maintenance"),
				"hardware_recovery_disabled": chRec.GetBool("hardware_recovery_disabled"),
			}
			if lockOwner, lockSecondsRemaining, lockHeld := h.sm.RecoveryLockStatus(chRec.Id); lockHeld {
				channel["hub_lock_owner"] = lockOwner
				channel["hub_lock_seconds_remaining"] = lockSecondsRemaining
			}
			channels = append(channels, channel)
		}
	}
	return e.JSON(http.StatusOK, map[string]any{
		"status":                           rec.GetString("status"),
		"config_revision":                  rec.GetInt("config_revision"),
		"config_hash":                      rec.GetString("config_hash"),
		"pending_esp_change":               rec.GetBool("pending_esp_change"),
		"name":                             rec.GetString("name"),
		"gateway_ip":                       rec.GetString("gateway_ip"),
		"gateway_name":                     rec.GetString("gateway_name"),
		"ping_interval_seconds":            rec.GetInt("ping_interval_seconds"),
		"temperature_monitoring_disabled":  rec.GetBool("temperature_monitoring_disabled"),
		"temp_threshold_warning":           rec.GetFloat("temp_threshold_warning"),
		"temp_threshold_critical":          rec.GetFloat("temp_threshold_critical"),
		"buzzer_disabled":                  rec.GetBool("buzzer_disabled"),
		"buzzer_muted":                     rec.GetBool("buzzer_muted"),
		"channels":                         channels,
	})
}

func (h *Hub) triggerRelayAction(e *core.RequestEvent, duration int, eventName string) error {
	systemID := e.Request.URL.Query().Get("system")
	if systemID == "" {
		return e.BadRequestError("Missing system ID", nil)
	}
	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("System not found or access denied", nil)
	}
	rec, err := e.App.FindFirstRecordByFilter("recovery_channels", "system = {:system}", dbx.Params{"system": systemID})
	if err != nil {
		return e.BadRequestError("Recovery is not configured for this system", err)
	}
	moduleID := rec.GetString("module")
	channelNum := rec.GetInt("channel_number")
	if moduleID == "" || channelNum <= 0 {
		return e.BadRequestError("Physical hardware watchdog is not configured for this system", nil)
	}
	moduleRec, err := e.App.FindRecordById("recovery_modules", moduleID)
	if err != nil {
		return e.NotFoundError("Mapped recovery module not found", err)
	}
	espIP := moduleRec.GetString("ip_address")
	if espIP == "" {
		return e.BadRequestError("Recovery module IP address is not available", nil)
	}
	leaseID, acquired := h.sm.AcquireRecoveryLock(rec.Id, "MANUAL_UI", 15*time.Second)
	if !acquired {
		return e.JSON(http.StatusConflict, map[string]any{
			"status":  http.StatusConflict,
			"message": "A recovery action is already in progress for this system. Please wait for it to complete.",
			"data":    map[string]any{},
		})
	}
	defer h.sm.ReleaseRecoveryLock(rec.Id, leaseID)
	err = h.sm.TriggerESP32Relay(espIP, channelNum, duration)
	if err != nil {
		return e.InternalServerError("Failed to contact ESP32 module relay controller", err)
	}
	collection, err := e.App.FindCollectionByNameOrId("recovery_events")
	if err == nil {
		eventRec := core.NewRecord(collection)
		eventRec.Set("system", systemID)
		eventRec.Set("module", moduleID)
		eventRec.Set("channel", channelNum)
		eventRec.Set("event", eventName)
		eventRec.Set("timestamp", time.Now().UTC())
		eventRec.Set("metadata", fmt.Sprintf(`{"module":"%s","channel":%d,"ip":"%s","source":"MANUAL_UI"}`, moduleID, channelNum, espIP))
		_ = e.App.Save(eventRec)
	}
	return e.JSON(http.StatusOK, map[string]any{"status": "ok"})
}

// triggerManualRelay handles POST /api/beszel/recovery/relay requests.
func (h *Hub) triggerManualRelay(e *core.RequestEvent) error {
	return h.triggerRelayAction(e, 500, "RELAY_MANUAL_SENT")
}

// triggerManualShutdown handles POST /api/beszel/recovery/shutdown requests.
func (h *Hub) triggerManualShutdown(e *core.RequestEvent) error {
	return h.triggerRelayAction(e, 300, "RELAY_SHUTDOWN_SENT")
}

// triggerManualForceRestart handles POST /api/beszel/recovery/force-restart requests.
func (h *Hub) triggerManualForceRestart(e *core.RequestEvent) error {
	return h.triggerRelayAction(e, 8000, "RELAY_FORCE_RESTART_SENT")
}

// refreshZfsData handles POST /api/beszel/zfs/refresh requests
// Fetches fresh ZFS detail data from the agent and updates the collection
func (h *Hub) refreshZfsData(e *core.RequestEvent) error {
	systemID := e.Request.URL.Query().Get("system")
	if systemID == "" {
		return e.BadRequestError("Invalid system parameter", nil)
	}

	system, err := h.sm.GetSystem(systemID)
	if err != nil || !system.HasUser(e.App, e.Auth) {
		return e.NotFoundError("", nil)
	}

	if err := system.FetchAndSaveZfsPools(true); err != nil {
		return e.InternalServerError("", err)
	}

	return e.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
