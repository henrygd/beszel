package main

import (
	_ "beszel/migrations"
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
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

var Version = "0.0.1-alpha.6"

var app *pocketbase.PocketBase
var serverConnections = make(map[string]*Server)
var serverConnectionsLock = sync.Mutex{}

func main() {
	app = pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: "beszel_data",
	})
	app.RootCmd.Version = Version
	app.RootCmd.Use = "beszel"
	app.RootCmd.Short = ""

	// add update command
	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update beszel to the latest version",
		Run:   updateBeszel,
	})

	// loosely check if it was executed using "go run"
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())

	// // enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		// (the isGoRun check is to enable it only during development)
		Automigrate: isGoRun,
	})

	// set auth settings
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		usersCollection, err := app.Dao().FindCollectionByNameOrId("users")
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
		if err := app.Dao().SaveCollection(usersCollection); err != nil {
			return err
		}
		return nil
	})

	// serve site
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
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
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// 15 second ticker for system updates
		go startSystemUpdateTicker()
		// cron job to delete old records
		scheduler := cron.New()
		scheduler.MustAdd("delete old records", "8 * * * *", func() {
			deleteOldRecords("system_stats", "1m", time.Hour)
			deleteOldRecords("container_stats", "1m", time.Hour)
			deleteOldRecords("system_stats", "10m", 12*time.Hour)
			deleteOldRecords("container_stats", "10m", 12*time.Hour)
			deleteOldRecords("system_stats", "20m", 24*time.Hour)
			deleteOldRecords("container_stats", "20m", 24*time.Hour)
			deleteOldRecords("system_stats", "120m", 7*24*time.Hour)
			deleteOldRecords("container_stats", "120m", 7*24*time.Hour)
			deleteOldRecords("system_stats", "480m", 30*24*time.Hour)
			deleteOldRecords("container_stats", "480m", 30*24*time.Hour)
		})
		scheduler.Start()
		return nil
	})

	// ssh key setup
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// create ssh key if it doesn't exist
		getSSHKey()
		// api route to return public key
		e.Router.GET("/api/beszel/getkey", func(c echo.Context) error {
			requestData := apis.RequestInfo(c)
			if requestData.AuthRecord == nil {
				return apis.NewForbiddenError("Forbidden", nil)
			}
			key, err := os.ReadFile(app.DataDir() + "/id_ed25519.pub")
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]string{"key": strings.TrimSuffix(string(key), "\n")})
		})
		return nil
	})

	// other api routes
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// check if first time setup on login page
		e.Router.GET("/api/beszel/first-run", func(c echo.Context) error {
			adminNum, err := app.Dao().TotalAdmins()
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]bool{"firstRun": adminNum == 0})
		})
		return nil
	})

	// immediately create connection for new servers
	app.OnModelAfterCreate("systems").Add(func(e *core.ModelEvent) error {
		go updateSystem(e.Model.(*models.Record))
		return nil
	})

	// do things after a systems record is updated
	app.OnModelAfterUpdate("systems").Add(func(e *core.ModelEvent) error {
		newRecord := e.Model.(*models.Record)
		oldRecord := newRecord.OriginalCopy()
		newStatus := newRecord.Get("status").(string)

		// if server is disconnected and connection exists, remove it
		if newStatus == "down" || newStatus == "paused" {
			deleteServerConnection(newRecord)
		}

		// if server is set to pending (unpause), try to connect immediately
		if newStatus == "pending" {
			go updateSystem(newRecord)
		}

		// alerts
		handleSystemAlerts(newStatus, newRecord, oldRecord)
		return nil
	})

	// do things after a systems record is deleted
	app.OnModelAfterDelete("systems").Add(func(e *core.ModelEvent) error {
		// if server connection exists, close it
		deleteServerConnection(e.Model.(*models.Record))
		return nil
	})

	app.OnModelAfterCreate("system_stats").Add(func(e *core.ModelEvent) error {
		createLongerRecords("system_stats", e.Model.(*models.Record))
		return nil
	})

	app.OnModelAfterCreate("container_stats").Add(func(e *core.ModelEvent) error {
		createLongerRecords("container_stats", e.Model.(*models.Record))
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func startSystemUpdateTicker() {
	ticker := time.NewTicker(15 * time.Second)
	for range ticker.C {
		updateSystems()
	}
}

func updateSystems() {
	records, err := app.Dao().FindRecordsByFilter(
		"2hz5ncl8tizk5nx",    // collection
		"status != 'paused'", // filter
		"updated",            // sort
		-1,                   // limit
		0,                    // offset
	)
	// log.Println("records", len(records))
	if err != nil {
		app.Logger().Error("Failed to query systems: ", "err", err.Error())
		return
	}
	fiftyFiveSecondsAgo := time.Now().UTC().Add(-55 * time.Second)
	batchSize := len(records)/4 + 1
	for i := 0; i < batchSize; i++ {
		if records[i].Get("updated").(types.DateTime).Time().After(fiftyFiveSecondsAgo) {
			break
		}
		// log.Println("updating", records[i].Get(("name")))
		go updateSystem(records[i])
	}
}

func updateSystem(record *models.Record) {
	var server *Server
	// check if server connection data exists
	if _, ok := serverConnections[record.Id]; ok {
		server = serverConnections[record.Id]
	} else {
		// create server connection struct
		server = &Server{
			Host: record.Get("host").(string),
			Port: record.Get("port").(string),
		}
		client, err := getServerConnection(server)
		if err != nil {
			app.Logger().Error("Failed to connect:", "err", err.Error(), "server", server.Host, "port", server.Port)
			updateServerStatus(record, "down")
			return
		}
		server.Client = client
		serverConnectionsLock.Lock()
		serverConnections[record.Id] = server
		serverConnectionsLock.Unlock()
	}
	// get server stats from agent
	systemData, err := requestJson(server)
	if err != nil {
		if err.Error() == "retry" {
			// if previous connection was closed, try again
			app.Logger().Error("Existing SSH connection closed. Retrying...", "host", server.Host, "port", server.Port)
			deleteServerConnection(record)
			updateSystem(record)
			return
		}
		app.Logger().Error("Failed to get server stats: ", "err", err.Error())
		updateServerStatus(record, "down")
		return
	}
	// update system record
	record.Set("status", "up")
	record.Set("info", systemData.Info)
	if err := app.Dao().SaveRecord(record); err != nil {
		app.Logger().Error("Failed to update record: ", "err", err.Error())
	}
	// add new system_stats record
	system_stats, _ := app.Dao().FindCollectionByNameOrId("system_stats")
	system_stats_record := models.NewRecord(system_stats)
	system_stats_record.Set("system", record.Id)
	system_stats_record.Set("stats", systemData.Stats)
	system_stats_record.Set("type", "1m")
	if err := app.Dao().SaveRecord(system_stats_record); err != nil {
		app.Logger().Error("Failed to save record: ", "err", err.Error())
	}
	// add new container_stats record
	if len(systemData.Containers) > 0 {
		container_stats, _ := app.Dao().FindCollectionByNameOrId("container_stats")
		container_stats_record := models.NewRecord(container_stats)
		container_stats_record.Set("system", record.Id)
		container_stats_record.Set("stats", systemData.Containers)
		container_stats_record.Set("type", "1m")
		if err := app.Dao().SaveRecord(container_stats_record); err != nil {
			app.Logger().Error("Failed to save record: ", "err", err.Error())
		}
	}
}

// set server to status down and close connection
func updateServerStatus(record *models.Record, status string) {
	// if in map, close connection and remove from map
	// this is now down automatically in an after update hook
	// if status == "down" || status == "paused" {
	// 	deleteServerConnection(record)
	// }
	if record.Get("status") != status {
		record.Set("status", status)
		if err := app.Dao().SaveRecord(record); err != nil {
			app.Logger().Error("Failed to update record: ", "err", err.Error())
		}
	}
}

func deleteServerConnection(record *models.Record) {
	if _, ok := serverConnections[record.Id]; ok {
		if serverConnections[record.Id].Client != nil {
			serverConnections[record.Id].Client.Close()
		}
		serverConnectionsLock.Lock()
		defer serverConnectionsLock.Unlock()
		delete(serverConnections, record.Id)
	}
}

func getServerConnection(server *Server) (*ssh.Client, error) {
	// app.Logger().Debug("new ssh connection", "server", server.Host)
	key, err := getSSHKey()
	if err != nil {
		app.Logger().Error("Failed to get SSH key: ", "err", err.Error())
		return nil, err
	}
	time.Sleep(time.Second)

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

func requestJson(server *Server) (SystemData, error) {
	session, err := server.Client.NewSession()
	if err != nil {
		return SystemData{}, errors.New("retry")
	}
	defer session.Close()

	// Create a buffer to capture the output
	var outputBuffer bytes.Buffer
	session.Stdout = &outputBuffer

	if err := session.Shell(); err != nil {
		return SystemData{}, err
	}

	err = session.Wait()
	if err != nil {
		return SystemData{}, err
	}

	// Unmarshal the output into our struct
	var systemData SystemData
	err = json.Unmarshal(outputBuffer.Bytes(), &systemData)
	if err != nil {
		return SystemData{}, err
	}

	return systemData, nil
}

func getSSHKey() ([]byte, error) {
	dataDir := app.DataDir()
	// check if the key pair already exists
	existingKey, err := os.ReadFile(dataDir + "/id_ed25519")
	if err == nil {
		return existingKey, nil
	}

	// Generate the Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		// app.Logger().Error("Error generating key pair:", "err", err.Error())
		return nil, err
	}

	// Get the private key in OpenSSH format
	privKeyBytes, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		// app.Logger().Error("Error marshaling private key:", "err", err.Error())
		return nil, err
	}

	// Save the private key to a file
	privateFile, err := os.Create(dataDir + "/id_ed25519")
	if err != nil {
		// app.Logger().Error("Error creating private key file:", "err", err.Error())
		return nil, err
	}
	defer privateFile.Close()

	if err := pem.Encode(privateFile, privKeyBytes); err != nil {
		// app.Logger().Error("Error writing private key to file:", "err", err.Error())
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

	app.Logger().Info("ed25519 SSH key pair generated successfully.")
	app.Logger().Info("Private key saved to: " + dataDir + "/id_ed25519")
	app.Logger().Info("Public key saved to: " + dataDir + "/id_ed25519.pub")

	existingKey, err = os.ReadFile(dataDir + "/id_ed25519")
	if err == nil {
		return existingKey, nil
	}
	return nil, err
}
