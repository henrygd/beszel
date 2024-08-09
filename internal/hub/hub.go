package hub

import (
	"beszel/internal/alerts"
	"beszel/internal/entities/server"
	"beszel/internal/entities/system"
	"beszel/internal/records"
	"beszel/site"
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

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
	app                   *pocketbase.PocketBase
	serverConnectionsLock *sync.Mutex
	serverConnections     map[string]*server.Server
}

func NewHub(app *pocketbase.PocketBase) *Hub {
	return &Hub{
		app:                   app,
		serverConnectionsLock: &sync.Mutex{},
		serverConnections:     make(map[string]*server.Server),
	}
}

func (h *Hub) Run() {

	rm := records.NewRecordManager(h.app)
	am := alerts.NewAlertManager(h.app)

	// loosely check if it was executed using "go run"
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())

	// // enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(h.app, h.app.RootCmd, migratecmd.Config{
		// (the isGoRun check is to enable it only during development)
		Automigrate: isGoRun,
	})

	// set auth settings
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
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

	// serve site
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		switch isGoRun {
		case true:
			proxy := httputil.NewSingleHostReverseProxy(&url.URL{
				Scheme: "http",
				Host:   "localhost:5173",
			})
			e.Router.GET("/static/*", apis.StaticDirectoryHandler(os.DirFS("./site/public/static"), false))
			e.Router.Any("/*", echo.WrapHandler(proxy))
			// e.Router.Any("/", echo.WrapHandler(proxy))
		default:
			e.Router.GET("/static/*", apis.StaticDirectoryHandler(site.Static, false))
			e.Router.Any("/*", apis.StaticDirectoryHandler(site.Dist, true))
		}
		return nil
	})

	// set up cron jobs / ticker for system updates
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// 15 second ticker for system updates
		go h.startSystemUpdateTicker()
		// cron job to delete old records
		scheduler := cron.New()
		scheduler.MustAdd("delete old records", "8 * * * *", func() {
			collections := []string{"system_stats", "container_stats"}
			rm.DeleteOldRecords(collections, "1m", time.Hour)
			rm.DeleteOldRecords(collections, "10m", 12*time.Hour)
			rm.DeleteOldRecords(collections, "20m", 24*time.Hour)
			rm.DeleteOldRecords(collections, "120m", 7*24*time.Hour)
			rm.DeleteOldRecords(collections, "480m", 30*24*time.Hour)
		})
		scheduler.Start()
		return nil
	})

	// ssh key setup
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// create ssh key if it doesn't exist
		h.getSSHKey()
		// api route to return public key
		e.Router.GET("/api/beszel/getkey", func(c echo.Context) error {
			requestData := apis.RequestInfo(c)
			if requestData.AuthRecord == nil {
				return apis.NewForbiddenError("Forbidden", nil)
			}
			key, err := os.ReadFile(h.app.DataDir() + "/id_ed25519.pub")
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]string{"key": strings.TrimSuffix(string(key), "\n")})
		})
		return nil
	})

	// other api routes
	h.app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// check if first time setup on login page
		e.Router.GET("/api/beszel/first-run", func(c echo.Context) error {
			adminNum, err := h.app.Dao().TotalAdmins()
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]bool{"firstRun": adminNum == 0})
		})
		return nil
	})

	// user creation - set default role to user if unset
	h.app.OnModelBeforeCreate("users").Add(func(e *core.ModelEvent) error {
		user := e.Model.(*models.Record)
		if user.GetString("role") == "" {
			user.Set("role", "user")
		}
		return nil
	})

	// system creation defaults
	h.app.OnModelBeforeCreate("systems").Add(func(e *core.ModelEvent) error {
		record := e.Model.(*models.Record)
		record.Set("info", system.SystemInfo{})
		record.Set("status", "pending")
		return nil
	})

	// immediately create connection for new servers
	h.app.OnModelAfterCreate("systems").Add(func(e *core.ModelEvent) error {
		go h.updateSystem(e.Model.(*models.Record))
		return nil
	})

	// do things after a systems record is updated
	h.app.OnModelAfterUpdate("systems").Add(func(e *core.ModelEvent) error {
		newRecord := e.Model.(*models.Record)
		oldRecord := newRecord.OriginalCopy()
		newStatus := newRecord.GetString("status")

		// if server is disconnected and connection exists, remove it
		if newStatus == "down" || newStatus == "paused" {
			h.deleteServerConnection(newRecord)
		}

		// if server is set to pending (unpause), try to connect immediately
		if newStatus == "pending" {
			go h.updateSystem(newRecord)
		}

		// alerts
		am.HandleSystemAlerts(newStatus, newRecord, oldRecord)
		return nil
	})

	// do things after a systems record is deleted
	h.app.OnModelAfterDelete("systems").Add(func(e *core.ModelEvent) error {
		// if server connection exists, close it
		h.deleteServerConnection(e.Model.(*models.Record))
		return nil
	})

	h.app.OnModelAfterCreate("system_stats").Add(func(e *core.ModelEvent) error {
		rm.CreateLongerRecords("system_stats", e.Model.(*models.Record))
		return nil
	})

	h.app.OnModelAfterCreate("container_stats").Add(func(e *core.ModelEvent) error {
		rm.CreateLongerRecords("container_stats", e.Model.(*models.Record))
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
		"2hz5ncl8tizk5nx",    // collection
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
	var s *server.Server
	// check if server connection data exists
	if _, ok := h.serverConnections[record.Id]; ok {
		s = h.serverConnections[record.Id]
	} else {
		// create server connection struct
		s = server.NewServer(
			record.GetString("host"),
			record.GetString("port"))
		client, err := h.getServerConnection(s)
		if err != nil {
			h.app.Logger().Error("Failed to connect:", "err", err.Error(), "server", s.Host, "port", s.Port)
			h.updateServerStatus(record, "down")
			return
		}
		s.Client = client
		h.serverConnectionsLock.Lock()
		h.serverConnections[record.Id] = s
		h.serverConnectionsLock.Unlock()
	}
	// get server stats from agent
	systemData, err := requestJson(s)
	if err != nil {
		if err.Error() == "retry" {
			// if previous connection was closed, try again
			h.app.Logger().Error("Existing SSH connection closed. Retrying...", "host", s.Host, "port", s.Port)
			h.deleteServerConnection(record)
			h.updateSystem(record)
			return
		}
		h.app.Logger().Error("Failed to get server stats: ", "err", err.Error())
		h.updateServerStatus(record, "down")
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
	system_stats_record := models.NewRecord(system_stats)
	system_stats_record.Set("system", record.Id)
	system_stats_record.Set("stats", systemData.Stats)
	system_stats_record.Set("type", "1m")
	if err := h.app.Dao().SaveRecord(system_stats_record); err != nil {
		h.app.Logger().Error("Failed to save record: ", "err", err.Error())
	}
	// add new container_stats record
	if len(systemData.Containers) > 0 {
		container_stats, _ := h.app.Dao().FindCollectionByNameOrId("container_stats")
		container_stats_record := models.NewRecord(container_stats)
		container_stats_record.Set("system", record.Id)
		container_stats_record.Set("stats", systemData.Containers)
		container_stats_record.Set("type", "1m")
		if err := h.app.Dao().SaveRecord(container_stats_record); err != nil {
			h.app.Logger().Error("Failed to save record: ", "err", err.Error())
		}
	}
}

// set server to status down and close connection
func (h *Hub) updateServerStatus(record *models.Record, status string) {
	// if in map, close connection and remove from map
	// this is now down automatically in an after update hook
	// if status == "down" || status == "paused" {
	// 	deleteServerConnection(record)
	// }
	if record.GetString("status") != status {
		record.Set("status", status)
		if err := h.app.Dao().SaveRecord(record); err != nil {
			h.app.Logger().Error("Failed to update record: ", "err", err.Error())
		}
	}
}

func (h *Hub) deleteServerConnection(record *models.Record) {
	if _, ok := h.serverConnections[record.Id]; ok {
		if h.serverConnections[record.Id].Client != nil {
			h.serverConnections[record.Id].Client.Close()
		}
		h.serverConnectionsLock.Lock()
		defer h.serverConnectionsLock.Unlock()
		delete(h.serverConnections, record.Id)
	}
}

func (h *Hub) getServerConnection(server *server.Server) (*ssh.Client, error) {
	// h.app.Logger().Debug("new ssh connection", "server", server.Host)
	key, err := h.getSSHKey()
	if err != nil {
		h.app.Logger().Error("Failed to get SSH key: ", "err", err.Error())
		return nil, err
	}

	// Create the Signer for this private key.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: "u",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", server.Host, server.Port), config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func requestJson(server *server.Server) (system.SystemData, error) {
	session, err := server.Client.NewSession()
	if err != nil {
		return system.SystemData{}, errors.New("retry")
	}
	defer session.Close()

	// Create a buffer to capture the output
	var outputBuffer bytes.Buffer
	session.Stdout = &outputBuffer

	if err := session.Shell(); err != nil {
		return system.SystemData{}, err
	}

	err = session.Wait()
	if err != nil {
		return system.SystemData{}, err
	}

	// Unmarshal the output into our struct
	var systemData system.SystemData
	err = json.Unmarshal(outputBuffer.Bytes(), &systemData)
	if err != nil {
		return system.SystemData{}, err
	}

	return systemData, nil
}

func (h *Hub) getSSHKey() ([]byte, error) {
	dataDir := h.app.DataDir()
	// check if the key pair already exists
	existingKey, err := os.ReadFile(dataDir + "/id_ed25519")
	if err == nil {
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