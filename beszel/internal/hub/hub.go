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
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"golang.org/x/crypto/ssh"
)

type Hub struct {
	app               *pocketbase.PocketBase
	systemConnections sync.Map
	sshClientConfig   *ssh.ClientConfig
	pubKey            string
	am                *alerts.AlertManager
	um                *users.UserManager
	rm                *records.RecordManager
	systemStats       *core.Collection
	containerStats    *core.Collection
}

func NewHub(app *pocketbase.PocketBase) *Hub {
	return &Hub{
		app: app,
		am:  alerts.NewAlertManager(app),
		um:  users.NewUserManager(app),
		rm:  records.NewRecordManager(app),
	}
}

func (h *Hub) Run() {
	// loosely check if it was executed using "go run"
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())

	// enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(h.app, h.app.RootCmd, migratecmd.Config{
		// (the isGoRun check is to enable it only during development)
		Automigrate: isGoRun,
		Dir:         "../../migrations",
	})

	// initial setup
	h.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// create ssh client config
		err := h.createSSHClientConfig()
		if err != nil {
			log.Fatal(err)
		}
		// set general settings
		settings := h.app.Settings()
		// batch requests (for global alerts)
		settings.Batch.Enabled = true
		// set auth settings
		usersCollection, err := h.app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		// disable email auth if DISABLE_PASSWORD_AUTH env var is set
		usersCollection.PasswordAuth.Enabled = os.Getenv("DISABLE_PASSWORD_AUTH") != "true"
		usersCollection.PasswordAuth.IdentityFields = []string{"email"}
		// disable oauth if no providers are configured (todo: remove this in post 0.9.0 release)
		if usersCollection.OAuth2.Enabled {
			usersCollection.OAuth2.Enabled = len(usersCollection.OAuth2.Providers) > 0
		}
		// allow oauth user creation if USER_CREATION is set
		if os.Getenv("USER_CREATION") == "true" {
			cr := "@request.context = 'oauth2'"
			usersCollection.CreateRule = &cr
		} else {
			usersCollection.CreateRule = nil
		}
		if err := h.app.Save(usersCollection); err != nil {
			return err
		}
		// sync systems with config
		h.syncSystemsWithConfig()
		return se.Next()
	})

	// serve web ui
	h.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		switch isGoRun {
		case true:
			proxy := httputil.NewSingleHostReverseProxy(&url.URL{
				Scheme: "http",
				Host:   "localhost:5173",
			})
			se.Router.Any("/", func(e *core.RequestEvent) error {
				proxy.ServeHTTP(e.Response, e.Request)
				return nil
			})
		default:
			csp, cspExists := os.LookupEnv("CSP")
			se.Router.Any("/{path...}", func(e *core.RequestEvent) error {
				if cspExists {
					e.Response.Header().Del("X-Frame-Options")
					e.Response.Header().Set("Content-Security-Policy", csp)
				}
				indexFallback := !strings.HasPrefix(e.Request.URL.Path, "/static/")
				return apis.Static(site.DistDirFS, indexFallback)(e)
			})
		}
		return se.Next()
	})

	// set up scheduled jobs / ticker for system updates
	h.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// 15 second ticker for system updates
		go h.startSystemUpdateTicker()
		// set up cron jobs
		// delete old records once every hour
		h.app.Cron().MustAdd("delete old records", "8 * * * *", h.rm.DeleteOldRecords)
		// create longer records every 10 minutes
		h.app.Cron().MustAdd("create longer records", "*/10 * * * *", func() {
			if systemStats, containerStats, err := h.getCollections(); err == nil {
				h.rm.CreateLongerRecords([]*core.Collection{systemStats, containerStats})
			}
		})
		return se.Next()
	})

	// custom api routes
	h.app.OnServe().BindFunc(func(se *core.ServeEvent) error {
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
			total, err := h.app.CountRecords("users")
			return e.JSON(http.StatusOK, map[string]bool{"firstRun": err == nil && total == 0})
		})
		// send test notification
		se.Router.GET("/api/beszel/send-test-notification", h.am.SendTestNotification)
		// API endpoint to get config.yml content
		se.Router.GET("/api/beszel/config-yaml", h.getYamlConfig)
		// create first user endpoint only needed if no users exist
		if totalUsers, _ := h.app.CountRecords("users"); totalUsers == 0 {
			se.Router.POST("/api/beszel/create-user", h.um.CreateFirstUser)
		}
		return se.Next()
	})

	// system creation defaults
	h.app.OnRecordCreate("systems").BindFunc(func(e *core.RecordEvent) error {
		e.Record.Set("info", system.Info{})
		e.Record.Set("status", "pending")
		return e.Next()
	})

	// immediately create connection for new systems
	h.app.OnRecordAfterCreateSuccess("systems").BindFunc(func(e *core.RecordEvent) error {
		go h.updateSystem(e.Record)
		return e.Next()
	})

	// handle default values for user / user_settings creation
	h.app.OnRecordCreate("users").BindFunc(h.um.InitializeUserRole)
	h.app.OnRecordCreate("user_settings").BindFunc(h.um.InitializeUserSettings)

	// empty info for systems that are paused
	h.app.OnRecordUpdate("systems").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("status") == "paused" {
			e.Record.Set("info", system.Info{})
		}
		return e.Next()
	})

	// do things after a systems record is updated
	h.app.OnRecordAfterUpdateSuccess("systems").BindFunc(func(e *core.RecordEvent) error {
		newRecord := e.Record.Fresh()
		oldRecord := newRecord.Original()
		newStatus := newRecord.GetString("status")

		// if system is disconnected and connection exists, remove it
		if newStatus == "down" || newStatus == "paused" {
			h.deleteSystemConnection(newRecord)
		}

		// if system is set to pending (unpause), try to connect immediately
		if newStatus == "pending" {
			go h.updateSystem(newRecord)
		} else {
			h.am.HandleStatusAlerts(newStatus, oldRecord)

		}
		return e.Next()
	})

	// if system is deleted, close connection
	h.app.OnRecordAfterDeleteSuccess("systems").BindFunc(func(e *core.RecordEvent) error {
		h.deleteSystemConnection(e.Record)
		return e.Next()
	})

	if err := h.app.Start(); err != nil {
		log.Fatal(err)
	}
}

func (h *Hub) startSystemUpdateTicker() {
	c := time.Tick(15 * time.Second)
	for range c {
		h.updateSystems()
	}
}

func (h *Hub) updateSystems() {
	records, err := h.app.FindRecordsByFilter(
		"2hz5ncl8tizk5nx",    // systems collection
		"status != 'paused'", // filter
		"updated",            // sort
		-1,                   // limit
		0,                    // offset
	)
	// log.Println("records", len(records))
	if err != nil || len(records) == 0 {
		// h.app.Logger().Error("Failed to query systems")
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
		go h.updateSystem(record)
	}
}

func (h *Hub) updateSystem(record *core.Record) {
	var client *ssh.Client
	var err error

	// check if system connection exists
	if existingClient, ok := h.systemConnections.Load(record.Id); ok {
		client = existingClient.(*ssh.Client)
	} else {
		// create system connection
		client, err = h.createSystemConnection(record)
		if err != nil {
			if record.GetString("status") != "down" {
				h.app.Logger().Error("Failed to connect:", "err", err.Error(), "system", record.GetString("host"), "port", record.GetString("port"))
				h.updateSystemStatus(record, "down")
			}
			return
		}
		h.systemConnections.Store(record.Id, client)
	}
	// get system stats from agent
	var systemData system.CombinedData
	if err := h.requestJsonFromAgent(client, &systemData); err != nil {
		if err.Error() == "bad client" {
			// if previous connection was closed, try again
			h.app.Logger().Error("Existing SSH connection closed. Retrying...", "host", record.GetString("host"), "port", record.GetString("port"))
			h.deleteSystemConnection(record)
			time.Sleep(time.Millisecond * 100)
			h.updateSystem(record)
			return
		}
		h.app.Logger().Error("Failed to get system stats: ", "err", err.Error())
		h.updateSystemStatus(record, "down")
		return
	}
	// update system record
	record.Set("status", "up")
	record.Set("info", systemData.Info)
	if err := h.app.SaveNoValidate(record); err != nil {
		h.app.Logger().Error("Failed to update record: ", "err", err.Error())
	}
	// add system_stats and container_stats records
	if systemStats, containerStats, err := h.getCollections(); err != nil {
		h.app.Logger().Error("Failed to get collections: ", "err", err.Error())
	} else {
		// add new system_stats record
		systemStatsRecord := core.NewRecord(systemStats)
		systemStatsRecord.Set("system", record.Id)
		systemStatsRecord.Set("stats", systemData.Stats)
		systemStatsRecord.Set("type", "1m")
		if err := h.app.SaveNoValidate(systemStatsRecord); err != nil {
			h.app.Logger().Error("Failed to save record: ", "err", err.Error())
		}
		// add new container_stats record
		if len(systemData.Containers) > 0 {
			containerStatsRecord := core.NewRecord(containerStats)
			containerStatsRecord.Set("system", record.Id)
			containerStatsRecord.Set("stats", systemData.Containers)
			containerStatsRecord.Set("type", "1m")
			if err := h.app.SaveNoValidate(containerStatsRecord); err != nil {
				h.app.Logger().Error("Failed to save record: ", "err", err.Error())
			}
		}
	}

	// system info alerts
	if err := h.am.HandleSystemAlerts(record, systemData.Info, systemData.Stats.Temperatures, systemData.Stats.ExtraFs); err != nil {
		h.app.Logger().Error("System alerts error", "err", err.Error())
	}
}

// return system_stats and container_stats collections
func (h *Hub) getCollections() (*core.Collection, *core.Collection, error) {
	if h.systemStats == nil {
		systemStats, err := h.app.FindCollectionByNameOrId("system_stats")
		if err != nil {
			return nil, nil, err
		}
		h.systemStats = systemStats
	}
	if h.containerStats == nil {
		containerStats, err := h.app.FindCollectionByNameOrId("container_stats")
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
		if err := h.app.SaveNoValidate(record); err != nil {
			h.app.Logger().Error("Failed to update record: ", "err", err.Error())
		}
	}
}

// delete system connection from map and close connection
func (h *Hub) deleteSystemConnection(record *core.Record) {
	if client, ok := h.systemConnections.Load(record.Id); ok {
		if sshClient := client.(*ssh.Client); sshClient != nil {
			sshClient.Close()
		}
		h.systemConnections.Delete(record.Id)
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
	key, err := h.getSSHKey()
	if err != nil {
		h.app.Logger().Error("Failed to get SSH key: ", "err", err.Error())
		return err
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return err
	}

	h.sshClientConfig = &ssh.ClientConfig{
		User: "u",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
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
		return fmt.Errorf("bad client")
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

func (h *Hub) getSSHKey() ([]byte, error) {
	dataDir := h.app.DataDir()
	// check if the key pair already exists
	existingKey, err := os.ReadFile(dataDir + "/id_ed25519")
	if err == nil {
		if pubKey, err := os.ReadFile(h.app.DataDir() + "/id_ed25519.pub"); err == nil {
			h.pubKey = strings.TrimSuffix(string(pubKey), "\n")
		}
		// return existing private key
		return existingKey, nil
	}

	// Generate the Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		// h.app.Logger().Error("Error generating key pair:", "err", err.Error())
		return nil, err
	}

	// Get the private key in OpenSSH format
	privKeyBytes, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		// h.app.Logger().Error("Error marshaling private key:", "err", err.Error())
		return nil, err
	}

	// Save the private key to a file
	privateFile, err := os.Create(dataDir + "/id_ed25519")
	if err != nil {
		// h.app.Logger().Error("Error creating private key file:", "err", err.Error())
		return nil, err
	}
	defer privateFile.Close()

	if err := pem.Encode(privateFile, privKeyBytes); err != nil {
		// h.app.Logger().Error("Error writing private key to file:", "err", err.Error())
		return nil, err
	}

	// Generate the public key in OpenSSH format
	publicKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, err
	}

	pubKeyBytes := ssh.MarshalAuthorizedKey(publicKey)
	h.pubKey = strings.TrimSuffix(string(pubKeyBytes), "\n")

	// Save the public key to a file
	publicFile, err := os.Create(dataDir + "/id_ed25519.pub")
	if err != nil {
		return nil, err
	}
	defer publicFile.Close()

	if _, err := publicFile.Write(pubKeyBytes); err != nil {
		return nil, err
	}

	h.app.Logger().Info("ed25519 SSH key pair generated successfully.")
	h.app.Logger().Info("Private key saved to: " + dataDir + "/id_ed25519")
	h.app.Logger().Info("Public key saved to: " + dataDir + "/id_ed25519.pub")

	existingKey, err = os.ReadFile(dataDir + "/id_ed25519")
	if err == nil {
		return existingKey, nil
	}
	return nil, err
}
