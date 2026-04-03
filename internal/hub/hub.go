// Package hub handles updating systems and serving the web UI.
package hub

import (
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/henrygd/beszel/internal/alerts"
	"github.com/henrygd/beszel/internal/hub/config"
	"github.com/henrygd/beszel/internal/hub/heartbeat"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/users"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

// Hub is the application. It embeds the PocketBase app and keeps references to subcomponents.
type Hub struct {
	core.App
	*alerts.AlertManager
	um     *users.UserManager
	rm     *records.RecordManager
	sm     *systems.SystemManager
	hb     *heartbeat.Heartbeat
	hbStop chan struct{}
	pubKey string
	signer ssh.Signer
	appURL string
}

// NewHub creates a new Hub instance with default configuration
func NewHub(app core.App) *Hub {
	hub := &Hub{App: app}
	hub.AlertManager = alerts.NewAlertManager(hub)
	hub.um = users.NewUserManager(hub)
	hub.rm = records.NewRecordManager(hub)
	hub.sm = systems.NewSystemManager(hub)
	hub.hb = heartbeat.New(app, GetEnv)
	if hub.hb != nil {
		hub.hbStop = make(chan struct{})
	}
	_ = onAfterBootstrapAndMigrations(app, hub.initialize)
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

// onAfterBootstrapAndMigrations ensures the provided function runs after the database is set up and migrations are applied.
// This is a workaround for behavior in PocketBase where onBootstrap runs before migrations, forcing use of onServe for this purpose.
// However, PB's tests.TestApp is already bootstrapped, generally doesn't serve, but does handle migrations.
// So this ensures that the provided function runs at the right time either way, after DB is ready and migrations are done.
func onAfterBootstrapAndMigrations(app core.App, fn func(app core.App) error) error {
	// pb tests.TestApp is already bootstrapped and doesn't serve
	if app.IsBootstrapped() {
		return fn(app)
	}
	// Must use OnServe because OnBootstrap appears to run before migrations, even if calling e.Next() before anything else
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := fn(e.App); err != nil {
			return err
		}
		return e.Next()
	})
	return nil
}

// StartHub sets up event handlers and starts the PocketBase server
func (h *Hub) StartHub() error {
	h.App.OnServe().BindFunc(func(e *core.ServeEvent) error {
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
		// start heartbeat if configured
		if h.hb != nil {
			go h.hb.Start(h.hbStop)
		}
		return e.Next()
	})

	// TODO: move to users package
	// handle default values for user / user_settings creation
	h.App.OnRecordCreate("users").BindFunc(h.um.InitializeUserRole)
	h.App.OnRecordCreate("user_settings").BindFunc(h.um.InitializeUserSettings)

	pb, ok := h.App.(*pocketbase.PocketBase)
	if !ok {
		return errors.New("not a pocketbase app")
	}
	return pb.Start()
}

// initialize sets up initial configuration (collections, settings, etc.)
func (h *Hub) initialize(app core.App) error {
	// set general settings
	settings := app.Settings()
	// batch requests (for alerts)
	settings.Batch.Enabled = true
	// set URL if APP_URL env is set
	if appURL, isSet := GetEnv("APP_URL"); isSet {
		h.appURL = appURL
		settings.Meta.AppURL = appURL
	}
	if err := app.Save(settings); err != nil {
		return err
	}
	// set auth settings
	return setCollectionAuthSettings(app)
}

// registerCronJobs sets up scheduled tasks
func (h *Hub) registerCronJobs(_ *core.ServeEvent) error {
	// delete old system_stats and alerts_history records once every hour
	h.Cron().MustAdd("delete old records", "8 * * * *", h.rm.DeleteOldRecords)
	// create longer records every 10 minutes
	h.Cron().MustAdd("create longer records", "*/10 * * * *", h.rm.CreateLongerRecords)
	return nil
}

// GetSSHKey generates key pair if it doesn't exist and returns signer
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
