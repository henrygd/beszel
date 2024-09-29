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
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/cron"
	"golang.org/x/crypto/ssh"
)

type Hub struct {
	app               *pocketbase.PocketBase
	connectionLock    *sync.Mutex
	systemConnections map[string]*ssh.Client
	sshClientConfig   *ssh.ClientConfig
	pubKey            string
	am                *alerts.AlertManager
	um                *users.UserManager
	rm                *records.RecordManager
}

func NewHub(app *pocketbase.PocketBase) *Hub {
	return &Hub{
		app:               app,
		connectionLock:    &sync.Mutex{},
		systemConnections: make(map[string]*ssh.Client),
		am:                alerts.NewAlertManager(app),
		um:                users.NewUserManager(app),
		rm:                records.NewRecordManager(app),
	}
}

func (h *Hub) Run() {
	// rm := records.NewRecordManager(h.app)
	// am := alerts.NewAlertManager(h.app)
	// um := users.NewUserManager(h.app)

	// loosely check if it was executed using "go run"
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())

	// // enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(h.app, h.app.RootCmd, migratecmd.Config{
		// (the isGoRun check is to enable it only during development)
		Automigrate: isGoRun,
		Dir:         "../../migrations",
	})

	// initial setup
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// create ssh client config
		err := h.createSSHClientConfig()
		if err != nil {
			log.Fatal(err)
		}
		// set auth settings
		usersCollection, err := h.app.Dao().FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		usersAuthOptions := usersCollection.AuthOptions()
		usersAuthOptions.AllowUsernameAuth = false
		if os.Getenv("DISABLE_PASSWORD_AUTH") == "true" {
			usersAuthOptions.AllowEmailAuth = false
		} else {
			usersAuthOptions.AllowEmailAuth = true
		}
		usersCollection.SetOptions(usersAuthOptions)
		if err := h.app.Dao().SaveCollection(usersCollection); err != nil {
			return err
		}
		return nil
	})

	// serve web ui
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		switch isGoRun {
		case true:
			proxy := httputil.NewSingleHostReverseProxy(&url.URL{
				Scheme: "http",
				Host:   "localhost:5173",
			})
			e.Router.Any("/*", echo.WrapHandler(proxy))
		default:
			csp, cspExists := os.LookupEnv("CSP")
			e.Router.Any("/*", func(c echo.Context) error {
				if cspExists {
					c.Response().Header().Del("X-Frame-Options")
					c.Response().Header().Set("Content-Security-Policy", csp)
				}
				indexFallback := !strings.HasPrefix(c.Request().URL.Path, "/static/")
				return apis.StaticDirectoryHandler(site.Dist, indexFallback)(c)
			})
		}
		return nil
	})

	// set up scheduled jobs / ticker for system updates
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// 15 second ticker for system updates
		go h.startSystemUpdateTicker()
		// set up cron jobs
		scheduler := cron.New()
		// delete old records once every hour
		scheduler.MustAdd("delete old records", "8 * * * *", h.rm.DeleteOldRecords)
		// create longer records every 10 minutes
		scheduler.MustAdd("create longer records", "*/10 * * * *", h.rm.CreateLongerRecords)
		scheduler.Start()
		return nil
	})

	// custom api routes
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// returns public key
		e.Router.GET("/api/beszel/getkey", func(c echo.Context) error {
			requestData := apis.RequestInfo(c)
			if requestData.AuthRecord == nil {
				return apis.NewForbiddenError("Forbidden", nil)
			}
			return c.JSON(http.StatusOK, map[string]string{"key": h.pubKey, "v": beszel.Version})
		})
		// check if first time setup on login page
		e.Router.GET("/api/beszel/first-run", func(c echo.Context) error {
			adminNum, err := h.app.Dao().TotalAdmins()
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]bool{"firstRun": adminNum == 0})
		})
		// send test notification
		e.Router.GET("/api/beszel/send-test-notification", h.am.SendTestNotification)
		return nil
	})

	// system creation defaults
	h.app.OnModelBeforeCreate("systems").Add(func(e *core.ModelEvent) error {
		record := e.Model.(*models.Record)
		record.Set("info", system.Info{})
		record.Set("status", "pending")
		return nil
	})

	// immediately create connection for new systems
	h.app.OnModelAfterCreate("systems").Add(func(e *core.ModelEvent) error {
		go h.updateSystem(e.Model.(*models.Record))
		return nil
	})

	// handle default values for user / user_settings creation
	h.app.OnModelBeforeCreate("users").Add(h.um.InitializeUserRole)
	h.app.OnModelBeforeCreate("user_settings").Add(h.um.InitializeUserSettings)

	// do things after a systems record is updated
	h.app.OnModelAfterUpdate("systems").Add(func(e *core.ModelEvent) error {
		newRecord := e.Model.(*models.Record)
		oldRecord := newRecord.OriginalCopy()
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

		return nil
	})

	// do things after a systems record is deleted
	h.app.OnModelAfterDelete("systems").Add(func(e *core.ModelEvent) error {
		// if system connection exists, close it
		h.deleteSystemConnection(e.Model.(*models.Record))
		return nil
	})

	if err := h.app.Start(); err != nil {
		log.Fatal(err)
	}
}

func (h *Hub) startSystemUpdateTicker() {
	ticker := time.NewTicker(15 * time.Second)
	for range ticker.C {
		h.updateSystems()
	}
}

func (h *Hub) updateSystems() {
	records, err := h.app.Dao().FindRecordsByFilter(
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

func (h *Hub) updateSystem(record *models.Record) {
	var client *ssh.Client
	var err error

	// check if system connection data exists
	if _, ok := h.systemConnections[record.Id]; ok {
		client = h.systemConnections[record.Id]
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
		h.connectionLock.Lock()
		h.systemConnections[record.Id] = client
		h.connectionLock.Unlock()
	}
	// get system stats from agent
	var systemData system.CombinedData
	if err := h.requestJsonFromAgent(client, &systemData); err != nil {
		if err.Error() == "bad client" {
			// if previous connection was closed, try again
			h.app.Logger().Error("Existing SSH connection closed. Retrying...", "host", record.GetString("host"), "port", record.GetString("port"))
			h.deleteSystemConnection(record)
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
	if err := h.app.Dao().SaveRecord(record); err != nil {
		h.app.Logger().Error("Failed to update record: ", "err", err.Error())
	}
	// add new system_stats record
	system_stats, _ := h.app.Dao().FindCollectionByNameOrId("system_stats")
	systemStatsRecord := models.NewRecord(system_stats)
	systemStatsRecord.Set("system", record.Id)
	systemStatsRecord.Set("stats", systemData.Stats)
	systemStatsRecord.Set("type", "1m")
	if err := h.app.Dao().SaveRecord(systemStatsRecord); err != nil {
		h.app.Logger().Error("Failed to save record: ", "err", err.Error())
	}
	// add new container_stats record
	if len(systemData.Containers) > 0 {
		container_stats, _ := h.app.Dao().FindCollectionByNameOrId("container_stats")
		containerStatsRecord := models.NewRecord(container_stats)
		containerStatsRecord.Set("system", record.Id)
		containerStatsRecord.Set("stats", systemData.Containers)
		containerStatsRecord.Set("type", "1m")
		if err := h.app.Dao().SaveRecord(containerStatsRecord); err != nil {
			h.app.Logger().Error("Failed to save record: ", "err", err.Error())
		}
	}
	// system info alerts (todo: temp alerts, extra fs alerts)
	h.am.HandleSystemInfoAlerts(record, systemData.Info)
}

// set system to specified status and save record
func (h *Hub) updateSystemStatus(record *models.Record, status string) {
	if record.GetString("status") != status {
		record.Set("status", status)
		if err := h.app.Dao().SaveRecord(record); err != nil {
			h.app.Logger().Error("Failed to update record: ", "err", err.Error())
		}
	}
}

func (h *Hub) deleteSystemConnection(record *models.Record) {
	if _, ok := h.systemConnections[record.Id]; ok {
		if h.systemConnections[record.Id] != nil {
			h.systemConnections[record.Id].Close()
		}
		h.connectionLock.Lock()
		defer h.connectionLock.Unlock()
		delete(h.systemConnections, record.Id)
	}
}

func (h *Hub) createSystemConnection(record *models.Record) (*ssh.Client, error) {
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", record.GetString("host"), record.GetString("port")), h.sshClientConfig)
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
		Timeout:         5 * time.Second,
	}
	return nil
}

// Fetches system stats from the agent and decodes the json data into the provided struct
func (h *Hub) requestJsonFromAgent(client *ssh.Client, systemData *system.CombinedData) error {
	session, err := newSessionWithTimeout(client, 5*time.Second)
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
