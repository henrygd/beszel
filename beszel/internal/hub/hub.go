// Package hub handles updating systems and serving the web UI.
package hub

import (
	"beszel"
	"beszel/internal/alerts"
	"beszel/internal/hub/config"
	"beszel/internal/hub/systems"
	"beszel/internal/hub/tailscale"
	"beszel/internal/records"
	"beszel/internal/users"
	"beszel/site"
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

type Hub struct {
	core.App
	*alerts.AlertManager
	um     *users.UserManager
	rm     *records.RecordManager
	sm     *systems.SystemManager
	tm     *tailscale.Manager
	pubKey string
	signer ssh.Signer
	appURL string
}

// NewHub creates a new Hub instance with default configuration
func NewHub(app core.App) *Hub {
	hub := &Hub{}
	hub.App = app

	hub.AlertManager = alerts.NewAlertManager(hub)
	hub.um = users.NewUserManager(hub)
	hub.rm = records.NewRecordManager(hub)
	hub.sm = systems.NewSystemManager(hub)
	hub.tm = tailscale.NewManager(hub)
	hub.appURL, _ = GetEnv("APP_URL")
	return hub
}

// GetEnv retrieves an environment variable with a "BESZEL_HUB_" prefix, or falls back to the unprefixed key.
func GetEnv(key string) (value string, exists bool) {
	if value, exists = os.LookupEnv("BESZEL_HUB_" + key); exists {
		return value, exists
	}
	// Fallback to the old unprefixed key
	return os.LookupEnv(key)
}

func (h *Hub) StartHub() error {
	h.App.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// initialize settings / collections
		if err := h.initialize(e); err != nil {
			return err
		}
		// sync systems with config
		if err := config.SyncSystems(e); err != nil {
			return err
		}
		// register api routes
		if err := h.registerApiRoutes(e); err != nil {
			return err
		}
		// register cron jobs
		if err := h.registerCronJobs(e); err != nil {
			return err
		}
		// start server
		if err := h.startServer(e); err != nil {
			return err
		}
		// start system updates
		if err := h.sm.Initialize(); err != nil {
			return err
		}
		// initialize Tailscale monitoring
		if err := h.tm.Initialize(); err != nil {
			return err
		}
		return e.Next()
	})

	// TODO: move to users package
	// handle default values for user / user_settings creation
	h.App.OnRecordCreate("users").BindFunc(h.um.InitializeUserRole)
	h.App.OnRecordCreate("user_settings").BindFunc(h.um.InitializeUserSettings)

	if pb, ok := h.App.(*pocketbase.PocketBase); ok {
		// log.Println("Starting pocketbase")
		err := pb.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

// initialize sets up initial configuration (collections, settings, etc.)
func (h *Hub) initialize(e *core.ServeEvent) error {
	// set general settings
	settings := e.App.Settings()
	// batch requests (for global alerts)
	settings.Batch.Enabled = true
	// set URL if BASE_URL env is set
	if h.appURL != "" {
		settings.Meta.AppURL = h.appURL
	}
	if err := e.App.Save(settings); err != nil {
		return err
	}
	// set auth settings
	usersCollection, err := e.App.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	// disable email auth if DISABLE_PASSWORD_AUTH env var is set
	disablePasswordAuth, _ := GetEnv("DISABLE_PASSWORD_AUTH")
	usersCollection.PasswordAuth.Enabled = disablePasswordAuth != "true"
	usersCollection.PasswordAuth.IdentityFields = []string{"email"}
	// disable oauth if no providers are configured (todo: remove this in post 0.9.0 release)
	if usersCollection.OAuth2.Enabled {
		usersCollection.OAuth2.Enabled = len(usersCollection.OAuth2.Providers) > 0
	}
	// allow oauth user creation if USER_CREATION is set
	if userCreation, _ := GetEnv("USER_CREATION"); userCreation == "true" {
		cr := "@request.context = 'oauth2'"
		usersCollection.CreateRule = &cr
	} else {
		usersCollection.CreateRule = nil
	}
	if err := e.App.Save(usersCollection); err != nil {
		return err
	}
	// allow all users to access systems if SHARE_ALL_SYSTEMS is set
	systemsCollection, err := e.App.FindCachedCollectionByNameOrId("systems")
	if err != nil {
		return err
	}
	shareAllSystems, _ := GetEnv("SHARE_ALL_SYSTEMS")
	systemsReadRule := "@request.auth.id != \"\""
	if shareAllSystems != "true" {
		// default is to only show systems that the user id is assigned to
		systemsReadRule += " && users.id ?= @request.auth.id"
	}
	updateDeleteRule := systemsReadRule + " && @request.auth.role != \"readonly\""
	systemsCollection.ListRule = &systemsReadRule
	systemsCollection.ViewRule = &systemsReadRule
	systemsCollection.UpdateRule = &updateDeleteRule
	systemsCollection.DeleteRule = &updateDeleteRule
	if err := e.App.Save(systemsCollection); err != nil {
		return err
	}
	return nil
}

// startServer sets up the server for Beszel
func (h *Hub) startServer(se *core.ServeEvent) error {
	// TODO: exclude dev server from production binary
	switch h.IsDev() {
	case true:
		proxy := httputil.NewSingleHostReverseProxy(&url.URL{
			Scheme: "http",
			Host:   "localhost:5173",
		})
		se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
			proxy.ServeHTTP(e.Response, e.Request)
			return nil
		})
	default:
		// parse app url
		parsedURL, err := url.Parse(h.appURL)
		if err != nil {
			return err
		}
		// fix base paths in html if using subpath
		basePath := strings.TrimSuffix(parsedURL.Path, "/") + "/"
		indexFile, _ := fs.ReadFile(site.DistDirFS, "index.html")
		indexContent := strings.ReplaceAll(string(indexFile), "./", basePath)
		indexContent = strings.Replace(indexContent, "{{V}}", beszel.Version, 1)
		indexContent = strings.Replace(indexContent, "{{HUB_URL}}", h.appURL, 1)
		// set up static asset serving
		staticPaths := [2]string{"/static/", "/assets/"}
		serveStatic := apis.Static(site.DistDirFS, false)
		// get CSP configuration
		csp, cspExists := GetEnv("CSP")
		// add route
		se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
			// serve static assets if path is in staticPaths
			for i := range staticPaths {
				if strings.Contains(e.Request.URL.Path, staticPaths[i]) {
					e.Response.Header().Set("Cache-Control", "public, max-age=2592000")
					return serveStatic(e)
				}
			}
			if cspExists {
				e.Response.Header().Del("X-Frame-Options")
				e.Response.Header().Set("Content-Security-Policy", csp)
			}
			return e.HTML(http.StatusOK, indexContent)
		})
	}
	return nil
}

// registerCronJobs sets up scheduled tasks
func (h *Hub) registerCronJobs(_ *core.ServeEvent) error {
	// delete old system_stats and alerts_history records once every hour
	h.Cron().MustAdd("delete old records", "8 * * * *", h.rm.DeleteOldRecords)
	// create longer records every 10 minutes
	h.Cron().MustAdd("create longer records", "*/10 * * * *", h.rm.CreateLongerRecords)
	// fetch Tailscale network data every 5 minutes
	h.Cron().MustAdd("fetch tailscale data", "*/5 * * * *", func() {
		if err := h.tm.FetchNetworkData(); err != nil {
			slog.Error("Failed to fetch Tailscale data", "error", err)
		}
	})
	return nil
}

// custom api routes
func (h *Hub) registerApiRoutes(se *core.ServeEvent) error {
	// returns public key and version
	se.Router.GET("/api/beszel/getkey", func(e *core.RequestEvent) error {
		info, _ := e.RequestInfo()
		if info.Auth == nil {
			return apis.NewForbiddenError("Forbidden", nil)
		}
		return e.JSON(http.StatusOK, map[string]string{"key": h.pubKey, "v": beszel.Version})
	})
	// check if first time setup on login page
	se.Router.GET("/api/beszel/first-run", func(e *core.RequestEvent) error {
		total, err := h.CountRecords("users")
		return e.JSON(http.StatusOK, map[string]bool{"firstRun": err == nil && total == 0})
	})
	// send test notification
	se.Router.GET("/api/beszel/send-test-notification", h.SendTestNotification)
	// API endpoint to get config.yml content
	se.Router.GET("/api/beszel/config-yaml", config.GetYamlConfig)
	// handle agent websocket connection
	se.Router.GET("/api/beszel/agent-connect", h.handleAgentConnect)
	// get or create universal tokens
	se.Router.GET("/api/beszel/universal-token", h.getUniversalToken)
	// Tailscale API endpoints
	se.Router.GET("/api/beszel/tailscale/network", h.getTailscaleNetwork)
	se.Router.GET("/api/beszel/tailscale/stats", h.getTailscaleStats)
	se.Router.GET("/api/beszel/tailscale/nodes", h.getTailscaleNodes)
	// create first user endpoint only needed if no users exist
	if totalUsers, _ := h.CountRecords("users"); totalUsers == 0 {
		se.Router.POST("/api/beszel/create-user", h.um.CreateFirstUser)
	}
	return nil
}

// Handler for universal token API endpoint (create, read, delete)
func (h *Hub) getUniversalToken(e *core.RequestEvent) error {
	info, err := e.RequestInfo()
	if err != nil || info.Auth == nil {
		return apis.NewForbiddenError("Forbidden", nil)
	}

	tokenMap := universalTokenMap.GetMap()
	userID := info.Auth.Id
	query := e.Request.URL.Query()
	token := query.Get("token")
	tokenSet := token != ""

	if !tokenSet {
		// return existing token if it exists
		if token, _, ok := tokenMap.GetByValue(userID); ok {
			return e.JSON(http.StatusOK, map[string]any{"token": token, "active": true})
		}
		// if no token is provided, generate a new one
		token = uuid.New().String()
	}
	response := map[string]any{"token": token}

	switch query.Get("enable") {
	case "1":
		tokenMap.Set(token, userID, time.Hour)
	case "0":
		tokenMap.RemovebyValue(userID)
	}
	_, response["active"] = tokenMap.GetOk(token)
	return e.JSON(http.StatusOK, response)
}

// generates key pair if it doesn't exist and returns signer
func (h *Hub) GetSSHKey(dataDir string) (ssh.Signer, error) {
	if h.signer != nil {
		return h.signer, nil
	}

	if dataDir == "" {
		dataDir = h.DataDir()
	}

	privateKeyPath := path.Join(dataDir, "id_ed25519")

	// check if the key pair already exists
	existingKey, err := os.ReadFile(privateKeyPath)
	if err == nil {
		private, err := ssh.ParsePrivateKey(existingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %s", err)
		}
		pubKeyBytes := ssh.MarshalAuthorizedKey(private.PublicKey())
		h.pubKey = strings.TrimSuffix(string(pubKeyBytes), "\n")
		return private, nil
	} else if !os.IsNotExist(err) {
		// File exists but couldn't be read for some other reason
		return nil, fmt.Errorf("failed to read %s: %w", privateKeyPath, err)
	}

	// Generate the Ed25519 key pair
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	privKeyPem, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(privKeyPem), 0600); err != nil {
		return nil, fmt.Errorf("failed to write private key to %q: err: %w", privateKeyPath, err)
	}

	// These are fine to ignore the errors on, as we've literally just created a crypto.PublicKey | crypto.Signer
	sshPrivate, _ := ssh.NewSignerFromSigner(privKey)
	pubKeyBytes := ssh.MarshalAuthorizedKey(sshPrivate.PublicKey())
	h.pubKey = strings.TrimSuffix(string(pubKeyBytes), "\n")

	h.Logger().Info("ed25519 key pair generated successfully.")
	h.Logger().Info("Saved to: " + privateKeyPath)

	return sshPrivate, err
}

// MakeLink formats a link with the app URL and path segments.
// Only path segments should be provided.
func (h *Hub) MakeLink(parts ...string) string {
	base := strings.TrimSuffix(h.Settings().Meta.AppURL, "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		base = fmt.Sprintf("%s/%s", base, url.PathEscape(part))
	}
	return base
}

// getTailscaleNetwork returns the current Tailscale network data
func (h *Hub) getTailscaleNetwork(e *core.RequestEvent) error {
	info, err := e.RequestInfo()
	if err != nil || info.Auth == nil {
		return apis.NewForbiddenError("Forbidden", nil)
	}

	if !h.tm.IsEnabled() {
		return e.JSON(http.StatusServiceUnavailable, map[string]string{"error": "Tailscale monitoring is disabled"})
	}

	network := h.tm.GetNetworkData()
	if network == nil {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "No Tailscale network data available"})
	}

	return e.JSON(http.StatusOK, network)
}

// getTailscaleStats returns the current Tailscale network statistics
func (h *Hub) getTailscaleStats(e *core.RequestEvent) error {
	info, err := e.RequestInfo()
	if err != nil || info.Auth == nil {
		return apis.NewForbiddenError("Forbidden", nil)
	}

	if !h.tm.IsEnabled() {
		return e.JSON(http.StatusServiceUnavailable, map[string]string{"error": "Tailscale monitoring is disabled"})
	}

	stats := h.tm.GetStats()
	if stats == nil {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "No Tailscale statistics available"})
	}

	return e.JSON(http.StatusOK, stats)
}

// getTailscaleNodes returns the list of Tailscale nodes
func (h *Hub) getTailscaleNodes(e *core.RequestEvent) error {
	info, err := e.RequestInfo()
	if err != nil || info.Auth == nil {
		return apis.NewForbiddenError("Forbidden", nil)
	}

	if !h.tm.IsEnabled() {
		return e.JSON(http.StatusServiceUnavailable, map[string]string{"error": "Tailscale monitoring is disabled"})
	}

	network := h.tm.GetNetworkData()
	if network == nil {
		return e.JSON(http.StatusNotFound, map[string]string{"error": "No Tailscale network data available"})
	}

	return e.JSON(http.StatusOK, network.Nodes)
}
