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
	"github.com/henrygd/beszel/internal/email"
	"github.com/henrygd/beszel/internal/hub/badges"
	"github.com/henrygd/beszel/internal/hub/bulk"
	"github.com/henrygd/beszel/internal/hub/config"
	"github.com/henrygd/beszel/internal/hub/domains"
	"github.com/henrygd/beszel/internal/hub/export"
	"github.com/henrygd/beszel/internal/hub/heartbeat"
	"github.com/henrygd/beszel/internal/hub/incidents"
	"github.com/henrygd/beszel/internal/hub/maintenance"
	"github.com/henrygd/beszel/internal/hub/monitors"
	"github.com/henrygd/beszel/internal/hub/notifications"
	"github.com/henrygd/beszel/internal/hub/settings"
	"github.com/henrygd/beszel/internal/hub/statuspages"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/users"

	"ganigeorgiev.com/pocketbase/pocketbase"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/ssh"
)

// Hub is the application. It embeds the PocketBase app and keeps references to subcomponents.
type Hub struct {
	core.App
	*alerts.AlertManager
	um             *users.UserManager
	rm             *records.RecordManager
	sm             *systems.SystemManager
	monSched       *monitors.Scheduler
	monAPI         *monitors.APIHandler
	domainSched    *domains.Scheduler
	domainAPI      *domains.APIHandler
	exportAPI      *export.APIHandler
	statusPageAPI  *statuspages.APIHandler
	maintenanceAPI *maintenance.APIHandler
	bulkAPI        *bulk.APIHandler
	incidentAPI    *incidents.APIHandler
	badgeAPI       *badges.APIHandler
	settingsAPI    *settings.APIHandler
	hb             *heartbeat.Heartbeat
	hbStop         chan struct{}
	pubKey         string
	signer         ssh.Signer
	appURL         string
	started        bool
}

// NewHub creates a new Hub instance with default configuration
func NewHub(app core.App) *Hub {
	hub := &Hub{App: app}
	hub.AlertManager = alerts.NewAlertManager(hub)
	hub.um = users.NewUserManager(hub)
	hub.rm = records.NewRecordManager(hub)
	hub.sm = systems.NewSystemManager(hub)
	hub.monSched = monitors.NewScheduler(app)
	hub.monSched.SetAlertCallback(func(userID, title, message, link, linkText string) {
		hub.AlertManager.SendAlert(alerts.AlertMessageData{
			UserID:   userID,
			Title:    title,
			Message:  message,
			Link:     link,
			LinkText: linkText,
		})
	})
	hub.monAPI = monitors.NewAPIHandler(app, hub.monSched)
	hub.domainSched = domains.NewScheduler(app)
	hub.domainSched.SetAlertCallback(func(userID, title, message, link, linkText string) {
		hub.AlertManager.SendAlert(alerts.AlertMessageData{
			UserID:   userID,
			Title:    title,
			Message:  message,
			Link:     link,
			LinkText: linkText,
		})
	})
	hub.domainAPI = domains.NewAPIHandler(app, hub.domainSched)
	hub.statusPageAPI = statuspages.NewAPIHandler(app)
	hub.maintenanceAPI = maintenance.NewAPIHandler(app)
	hub.bulkAPI = bulk.NewAPIHandler(app)
	hub.incidentAPI = incidents.NewAPIHandler(app)
	hub.badgeAPI = badges.NewAPIHandler(app)
	hub.settingsAPI = settings.NewAPIHandler(app)
	hub.exportAPI = export.NewAPIHandler(app)
	hub.hb = heartbeat.New(app, utils.GetEnv)
	if hub.hb != nil {
		hub.hbStop = make(chan struct{})
	}
	_ = onAfterBootstrapAndMigrations(app, hub.initialize)
	return hub
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
		// start system updates and background services only once
		if !h.started {
			h.started = true
			// start system updates
			if err := h.sm.Initialize(); err != nil {
				return err
			}
			// start agent-only server if configured (after system manager is ready)
			if err := h.startAgentServer(e); err != nil {
				return err
			}
			// start heartbeat if configured
			if h.hb != nil {
				go h.hb.Start(h.hbStop)
			}
			// start monitor scheduler
			if err := h.monSched.Start(); err != nil {
				return err
			}
			// start domain scheduler
			h.domainSched.Start()
			// bind monitor lifecycle hooks
			h.bindMonitorHooks()
			// bind domain lifecycle hooks
			h.bindDomainHooks()
		}
		// register monitor API routes
		h.monAPI.RegisterRoutes(e)
		// register domain API routes
		h.domainAPI.RegisterRoutes(e)
		// register status page API routes
		h.statusPageAPI.RegisterRoutes(e)
		// register maintenance API routes
		h.maintenanceAPI.RegisterRoutes(e)
		// register bulk API routes
		h.bulkAPI.RegisterRoutes(e)
		// register incident API routes
		h.incidentAPI.RegisterRoutes(e)
		// register badge API routes
		h.badgeAPI.RegisterRoutes(e)
		// register settings API routes
		h.settingsAPI.RegisterRoutes(e)
		// register export API routes
		h.exportAPI.RegisterRoutes(e)
		// register notification API routes
		notifications.RegisterRoutes(h.App, e)
		return e.Next()
	})

	// NOTE: consider moving user initialization into users package
	// handle default values for user / user_settings creation
	h.App.OnRecordAuthWithOAuth2Request("users").BindFunc(h.um.InitializeOAuthUserRole)
	h.App.OnRecordCreate("users").BindFunc(h.um.InitializeUserRole)
	h.App.OnRecordCreate("user_settings").BindFunc(h.um.InitializeUserSettings)

	bindNetworkMonitorsEvents(h)
	email.BindAuthEmails(h.App)

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
	settings.Batch.MaxRequests = 100
	settings.Batch.MaxBodySize = 1 << 20 // 1 MiB
	// set URL if APP_URL env is set
	if appURL, isSet := utils.GetEnv("APP_URL"); isSet {
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
	// cleanup old monitor heartbeats once a day (keep 30 days)
	h.Cron().MustAdd("cleanup old heartbeats", "0 0 * * *", func() {
		h.monSched.CleanupOldHeartbeats(30)
	})
	// check domain expiry daily at 1 AM
	h.Cron().MustAdd("check domains", "0 1 * * *", func() {
		h.domainSched.CheckAllDomains()
	})
	return nil
}

// bindMonitorHooks binds event hooks for monitor lifecycle management
func (h *Hub) bindMonitorHooks() {
	// On create - add to scheduler
	h.OnRecordCreate("monitors").BindFunc(func(e *core.RecordEvent) error {
		// Only add to scheduler if active
		if e.Record.GetBool("active") {
			h.monSched.AddMonitor(e.Record)
		}
		return e.Next()
	})

	// On update - update scheduler
	h.OnRecordAfterUpdateSuccess("monitors").BindFunc(func(e *core.RecordEvent) error {
		h.monSched.UpdateMonitor(e.Record)
		return e.Next()
	})

	// On delete - remove from scheduler
	h.OnRecordAfterDeleteSuccess("monitors").BindFunc(func(e *core.RecordEvent) error {
		h.monSched.RemoveMonitor(e.Record.Id)
		return e.Next()
	})
}

// bindDomainHooks binds event hooks for domain lifecycle management
func (h *Hub) bindDomainHooks() {
	// On create - perform initial lookup if active
	h.OnRecordAfterCreateSuccess("domains").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetBool("active") {
			h.domainSched.RefreshDomain(e.Record.Id)
		}
		return e.Next()
	})

	// Manual refresh and resume actions trigger lookups explicitly. Avoid
	// refreshing on every domain save because scheduler writes would loop.
}

// GetSSHKey generates key pair if it doesn't exist and returns signer
func (h *Hub) GetSSHKey(dataDir string) (ssh.Signer, error) {
	if dataDir == "" {
		dataDir = h.DataDir()
	}

	// Only cache the signer when using the default data directory
	isDefaultDir := dataDir == h.DataDir()
	if isDefaultDir && h.signer != nil {
		return h.signer, nil
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
		if isDefaultDir {
			h.signer = private
		}
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
	if isDefaultDir {
		h.signer = sshPrivate
	}

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
