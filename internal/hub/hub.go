// Package hub handles updating systems and serving the web UI.
package hub

import (
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/alerts"
	"github.com/henrygd/beszel/internal/hub/config"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/users"

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
		// register middlewares
		h.registerMiddlewares(e)
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

// registerCronJobs sets up scheduled tasks
func (h *Hub) registerCronJobs(_ *core.ServeEvent) error {
	// delete old system_stats and alerts_history records once every hour
	h.Cron().MustAdd("delete old records", "8 * * * *", h.rm.DeleteOldRecords)
	// create longer records every 10 minutes
	h.Cron().MustAdd("create longer records", "*/10 * * * *", h.rm.CreateLongerRecords)
	return nil
}

// custom middlewares
func (h *Hub) registerMiddlewares(se *core.ServeEvent) {
	// authenticate with trusted header
	if trustedHeader, _ := GetEnv("TRUSTED_AUTH_HEADER"); trustedHeader != "" {
		se.Router.BindFunc(func(e *core.RequestEvent) error {
			if e.Auth != nil {
				return e.Next()
			}
			trustedEmail := e.Request.Header.Get(trustedHeader)
			if trustedEmail == "" {
				return e.Next()
			}
			isAuthRefresh := e.Request.URL.Path == "/api/collections/users/auth-refresh" && e.Request.Method == http.MethodPost
			if !isAuthRefresh {
				authRecord, err := e.App.FindAuthRecordByEmail("users", trustedEmail)
				if err == nil {
					e.Auth = authRecord
				}
				return e.Next()
			}
			// if auth refresh endpoint, find user record directly and generate token
			user, err := e.App.FindFirstRecordByData("users", "email", trustedEmail)
			if err != nil {
				return e.Next()
			}
			e.Auth = user
			// need to set the authorization header for the client sdk to pick up the token
			if token, err := user.NewAuthToken(); err == nil {
				e.Request.Header.Set("Authorization", token)
			}
			return e.Next()
		})
	}
}

// custom api routes
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
	apiAuth.GET("/getkey", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, map[string]string{"key": h.pubKey, "v": beszel.Version})
	})
	// send test notification
	apiAuth.POST("/test-notification", h.SendTestNotification)
	// get config.yml content
	apiAuth.GET("/config-yaml", config.GetYamlConfig)
	// handle agent websocket connection
	apiNoAuth.GET("/agent-connect", h.handleAgentConnect)
	// get or create universal tokens
	apiAuth.GET("/universal-token", h.getUniversalToken)
	// update / delete user alerts
	apiAuth.POST("/user-alerts", alerts.UpsertUserAlerts)
	apiAuth.DELETE("/user-alerts", alerts.DeleteUserAlerts)

	return nil
}

// Handler for universal token API endpoint (create, read, delete)
func (h *Hub) getUniversalToken(e *core.RequestEvent) error {
	tokenMap := universalTokenMap.GetMap()
	userID := e.Auth.Id
	query := e.Request.URL.Query()
	token := query.Get("token")

	if token == "" {
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
