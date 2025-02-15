// Package hub handles updating systems and serving the web UI.
package hub

import (
	"beszel"
	"beszel/internal/alerts"
	"beszel/internal/entities/system"
	"beszel/internal/records"
	"beszel/internal/users"
	"beszel/site"
	"context"
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

type Hub struct {
	*pocketbase.PocketBase
	sshClientConfig *ssh.ClientConfig
	pubKey          string
	am              *alerts.AlertManager
	um              *users.UserManager
	rm              *records.RecordManager
	systemStats     *core.Collection
	containerStats  *core.Collection
	appURL          string
	sshListenAddr   string
}

// NewHub creates a new Hub instance with default configuration
func NewHub() *Hub {
	var hub Hub
	hub.PocketBase = pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: beszel.AppName + "_data",
	})

	hub.RootCmd.PersistentFlags().StringVarP(&hub.sshListenAddr, "sshd", "s", "0.0.0.0:45877", "defines where beszel will start an ssh server to listen for client connections")

	hub.RootCmd.Version = beszel.Version
	hub.RootCmd.Use = beszel.AppName
	hub.RootCmd.Short = ""
	// add update command
	hub.RootCmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update " + beszel.AppName + " to the latest version",
		Run:   Update,
	})

	hub.am = alerts.NewAlertManager(hub)
	hub.um = users.NewUserManager(hub)
	hub.rm = records.NewRecordManager(hub)
	hub.appURL, _ = GetEnv("APP_URL")
	return &hub
}

// GetEnv retrieves an environment variable with a "BESZEL_HUB_" prefix, or falls back to the unprefixed key.
func GetEnv(key string) (value string, exists bool) {
	if value, exists = os.LookupEnv("BESZEL_HUB_" + key); exists {
		return value, exists
	}
	// Fallback to the old unprefixed key
	return os.LookupEnv(key)
}

func (h *Hub) Run() {
	// loosely check if it was executed using "go run"
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())

	// enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(h, h.RootCmd, migratecmd.Config{
		// (the isGoRun check is to enable it only during development)
		Automigrate: isGoRun,
		Dir:         "../../migrations",
	})

	// initial setup
	h.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// create ssh client config
		err := h.createSSHClientConfig()
		if err != nil {
			log.Fatal(err)
		}
		// set general settings
		settings := h.Settings()
		// batch requests (for global alerts)
		settings.Batch.Enabled = true
		// set URL if BASE_URL env is set
		if h.appURL != "" {
			settings.Meta.AppURL = h.appURL
		}

		// set auth settings
		usersCollection, err := h.FindCollectionByNameOrId("users")
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
		if err := h.Save(usersCollection); err != nil {
			return err
		}
		// sync systems with config
		h.syncSystemsWithConfig()
		return se.Next()
	})

	// serve web ui
	h.OnServe().BindFunc(func(se *core.ServeEvent) error {
		switch isGoRun {
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
		return se.Next()
	})

	h.OnServe().BindFunc(func(se *core.ServeEvent) error {
		err := h.startSSHServer(h.sshListenAddr)
		if err != nil {
			return fmt.Errorf("ssh server failed to listen: %w", err)
		}
		return se.Next()
	})

	// TODO cleanup old records in registration
	// set up scheduled jobs / ticker for system updates
	h.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// 15 second ticker for system updates
		go h.startSystemUpdateTicker()
		// set up cron jobs
		// delete old records once every hour
		h.Cron().MustAdd("delete old records", "8 * * * *", h.rm.DeleteOldRecords)
		// create longer records every 10 minutes
		h.Cron().MustAdd("create longer records", "*/10 * * * *", func() {
			if systemStats, containerStats, err := h.getCollections(); err == nil {
				h.rm.CreateLongerRecords([]*core.Collection{systemStats, containerStats})
			}
		})
		return se.Next()
	})

	// custom api routes
	h.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// returns public key
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
		se.Router.GET("/api/beszel/send-test-notification", h.am.SendTestNotification)
		// API endpoint to get config.yml content
		se.Router.GET("/api/beszel/config-yaml", h.getYamlConfig)
		// create first user endpoint only needed if no users exist
		if totalUsers, _ := h.CountRecords("users"); totalUsers == 0 {
			se.Router.POST("/api/beszel/create-user", h.um.CreateFirstUser)
		}
		return se.Next()
	})

	// system creation defaults
	h.OnRecordCreate("systems").BindFunc(func(e *core.RecordEvent) error {
		e.Record.Set("info", system.Info{})
		e.Record.Set("status", "pending")
		return e.Next()
	})

	// immediately create connection for new systems
	h.OnRecordAfterCreateSuccess("systems").BindFunc(func(e *core.RecordEvent) error {
		clientType := e.Record.GetString("type")
		switch clientType {

		// empty string is for backwards compatiablity
		case "server", "":
			go h.pollSystem(e.Record)
		case "client":
			// After a client has been accepted by the user (or auto accepted) it will be added to the systems table and should be removed from the new_systems table.
			record, err := h.FindFirstRecordByData("new_systems", "fingerprint", e.Record.GetString("fingerprint"))
			if err != nil {
				return err
			}

			err = h.Delete(record)
			if err != nil {
				return err
			}

		default:
			h.Logger().Debug("Beszel had a system with unknown type", "type", clientType)
			return fmt.Errorf("unknown client system record type %q", clientType)
		}

		return e.Next()
	})

	// once a system has been added to blocked systems delete it from new_systems
	h.OnRecordAfterCreateSuccess("blocked_systems").BindFunc(func(e *core.RecordEvent) error {

		// After a client has been accepted by the user (or auto accepted) it will be added to the systems table and should be removed from the new_systems table.
		record, err := h.FindFirstRecordByData("new_systems", "fingerprint", e.Record.GetString("fingerprint"))
		if err != nil {
			return err
		}

		err = h.Delete(record)
		if err != nil {
			return err
		}

		return e.Next()
	})

	// handle default values for user / user_settings creation
	h.OnRecordCreate("users").BindFunc(h.um.InitializeUserRole)
	h.OnRecordCreate("user_settings").BindFunc(h.um.InitializeUserSettings)

	// empty info for systems that are paused
	h.OnRecordUpdate("systems").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("status") == "paused" {
			e.Record.Set("info", system.Info{})
		}
		return e.Next()
	})

	// do things after a systems record is updated
	h.OnRecordAfterUpdateSuccess("systems").BindFunc(func(e *core.RecordEvent) error {
		newRecord := e.Record.Fresh()
		oldRecord := newRecord.Original()
		newStatus := newRecord.GetString("status")

		// if system is not up and connection exists, remove it
		if newStatus != "up" {
			h.deleteSystemConnection(newRecord)
		}

		// if system is set to pending (unpause), try to connect immediately
		if newStatus == "pending" {
			if newRecord.GetString("type") == "server" {
				go h.pollSystem(newRecord)
			}
			// We do not have to do anything with our client type beszel connections, they will send data every 15+/-5 seconds
		} else {
			h.am.HandleStatusAlerts(newStatus, oldRecord)
		}

		return e.Next()
	})

	// if system is deleted, close connection
	h.OnRecordAfterDeleteSuccess("systems").BindFunc(func(e *core.RecordEvent) error {
		h.deleteSystemConnection(e.Record)
		return e.Next()
	})

	if err := h.Start(); err != nil {
		log.Fatal(err)
	}
}

func (h *Hub) startSystemUpdateTicker() {
	c := time.Tick(15 * time.Second)
	for range c {
		h.pollServerSystems()
	}
}

func (h *Hub) pollServerSystems() {
	records, err := h.FindRecordsByFilter(
		"2hz5ncl8tizk5nx",                        // systems collection
		"status != 'paused' && type == 'server'", // filter
		"updated",                                // sort
		-1,                                       // limit
		0,                                        // offset
	)
	// log.Println("records", len(records))
	if err != nil || len(records) == 0 {
		// h.Logger().Error("Failed to query systems")
		return
	}
	fiftySecondsAgo := time.Now().UTC().Add(-50 * time.Second)
	batchSize := len(records)/4 + 1
	done := 0
	for _, record := range records {
		// break if batch size reached or if the system was updated less than 50 seconds ago
		if done >= batchSize || record.GetDateTime("updated").Time().After(fiftySecondsAgo) {
			break
		}
		// don't increment for down systems to avoid them jamming the queue
		// because they're always first when sorted by least recently updated
		if record.GetString("status") != "down" {
			done++
		}
		go h.pollSystem(record)
	}
}

// pollSystem connects to a server type beszel client on a specified port and prompts it for a record
func (h *Hub) pollSystem(record *core.Record) {
	if record.GetString("type") != "server" {
		h.Logger().Debug("Beszel attempted to poll a non-server system")
		return
	}

	var client *ssh.Client
	var err error

	// check if system connection exists
	if existingClient, ok := h.Store().GetOk(record.Id); ok {
		client = existingClient.(*ssh.Client)
	} else {
		// create system connection
		client, err = h.createSystemConnection(record)
		if err != nil {
			if record.GetString("status") != "down" {
				h.Logger().Error("Failed to connect:", "err", err.Error(), "system", record.GetString("host"), "port", record.GetString("port"))
				h.updateSystemStatus(record, "down")
			}
			return
		}
		h.Store().Set(record.Id, client)
	}
	// get system stats from agent
	var systemData system.CombinedData
	if err := h.requestJsonFromAgent(client, &systemData); err != nil {
		if err.Error() == "bad client" {
			// if previous connection was closed, try again
			h.Logger().Error("Existing SSH connection closed. Retrying...", "host", record.GetString("host"), "port", record.GetString("port"))
			h.deleteSystemConnection(record)
			time.Sleep(time.Millisecond * 100)
			h.pollSystem(record)
			return
		}
		h.Logger().Error("Failed to get system stats: ", "err", err.Error())
		h.updateSystemStatus(record, "down")
		return
	}
	// update system record
	record.Set("status", "up")
	record.Set("info", systemData.Info)
	if err := h.SaveNoValidate(record); err != nil {
		h.Logger().Error("Failed to update record: ", "err", err.Error())
	}
	// add system_stats and container_stats records
	if systemStats, containerStats, err := h.getCollections(); err != nil {
		h.Logger().Error("Failed to get collections: ", "err", err.Error())
	} else {
		// add new system_stats record
		systemStatsRecord := core.NewRecord(systemStats)
		systemStatsRecord.Set("system", record.Id)
		systemStatsRecord.Set("stats", systemData.Stats)
		systemStatsRecord.Set("type", "1m")
		if err := h.SaveNoValidate(systemStatsRecord); err != nil {
			h.Logger().Error("Failed to save record: ", "err", err.Error())
		}
		// add new container_stats record
		if len(systemData.Containers) > 0 {
			containerStatsRecord := core.NewRecord(containerStats)
			containerStatsRecord.Set("system", record.Id)
			containerStatsRecord.Set("stats", systemData.Containers)
			containerStatsRecord.Set("type", "1m")
			if err := h.SaveNoValidate(containerStatsRecord); err != nil {
				h.Logger().Error("Failed to save record: ", "err", err.Error())
			}
		}
	}

	// system info alerts
	if err := h.am.HandleSystemAlerts(record, systemData.Info, systemData.Stats.Temperatures, systemData.Stats.ExtraFs); err != nil {
		h.Logger().Error("System alerts error", "err", err.Error())
	}
}

// return system_stats and container_stats collections
func (h *Hub) getCollections() (*core.Collection, *core.Collection, error) {
	if h.systemStats == nil {
		systemStats, err := h.FindCollectionByNameOrId("system_stats")
		if err != nil {
			return nil, nil, err
		}
		h.systemStats = systemStats
	}
	if h.containerStats == nil {
		containerStats, err := h.FindCollectionByNameOrId("container_stats")
		if err != nil {
			return nil, nil, err
		}
		h.containerStats = containerStats
	}
	return h.systemStats, h.containerStats, nil
}

// set system to specified status and save record
func (h *Hub) updateSystemStatus(record *core.Record, status string) {
	if record.Fresh().GetString("status") != status {
		record.Set("status", status)
		if err := h.SaveNoValidate(record); err != nil {
			h.Logger().Error("Failed to update record: ", "err", err.Error())
		}
	}
}

// delete system connection from map and close connection
func (h *Hub) deleteSystemConnection(record *core.Record) {
	if client, ok := h.Store().GetOk(record.Id); ok {
		if sshClient, ok := client.(*ssh.Client); ok && sshClient != nil {
			sshClient.Close()
		} else if sshClient, ok := client.(*ssh.ServerConn); ok && sshClient != nil {
			sshClient.Close()
		}

		h.Store().Remove(record.Id)
	}
}

func (h *Hub) createSystemConnection(record *core.Record) (*ssh.Client, error) {
	client, err := ssh.Dial("tcp", net.JoinHostPort(record.GetString("host"), record.GetString("port")), h.sshClientConfig)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (h *Hub) createSSHClientConfig() error {
	privateKey, err := h.getSSHKey()
	if err != nil {
		h.Logger().Error("Failed to get SSH key: ", "err", err.Error())
		return err
	}

	h.sshClientConfig = &ssh.ClientConfig{
		User: "u",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(privateKey),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         4 * time.Second,
	}
	return nil
}

// Fetches system stats from the agent and decodes the json data into the provided struct
func (h *Hub) requestJsonFromAgent(client *ssh.Client, systemData *system.CombinedData) error {
	session, err := newSessionWithTimeout(client, 4*time.Second)
	if err != nil {
		return fmt.Errorf("bad client: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	if err := session.Shell(); err != nil {
		return err
	}

	if err := json.NewDecoder(stdout).Decode(systemData); err != nil {
		return err
	}

	// wait for the session to complete
	if err := session.Wait(); err != nil {
		return err
	}

	return nil
}

// Adds timeout to SSH session creation to avoid hanging in case of network issues
func newSessionWithTimeout(client *ssh.Client, timeout time.Duration) (*ssh.Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// use goroutine to create the session
	sessionChan := make(chan *ssh.Session, 1)
	errChan := make(chan error, 1)
	go func() {
		if session, err := client.NewSession(); err != nil {
			errChan <- err
		} else {
			sessionChan <- session
		}
	}()

	select {
	case session := <-sessionChan:
		return session, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("session creation timed out")
	}
}

func (h *Hub) getSSHKey() (ssh.Signer, error) {
	privateKeyPath := path.Join(h.DataDir(), "id_ed25519")

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
	}

	// Generate the Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}

	// Get the private key in OpenSSH format
	privKeyPem, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(privKeyPem), 0600); err != nil {
		return nil, fmt.Errorf("failed to write private key to %q: err: %w", privateKeyPath, err)
	}

	// These are fine to ignore the errors on, as we've literally just created a crypto.PublicKey | crypto.Signer
	sshPubKey, _ := ssh.NewPublicKey(pubKey)
	sshPrivate, _ := ssh.NewSignerFromSigner(privKey)

	pubKeyBytes := ssh.MarshalAuthorizedKey(sshPubKey)
	h.pubKey = strings.TrimSuffix(string(pubKeyBytes), "\n")

	h.Logger().Info("ed25519 SSH key pair generated successfully.")
	h.Logger().Info("Saved to: " + privateKeyPath)

	return sshPrivate, err
}
