package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	_ "monitor-site/migrations"
	"net/http"
	"net/http/httputil"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/cron"
	"github.com/pocketbase/pocketbase/tools/mailer"
	"golang.org/x/crypto/ssh"
)

var app *pocketbase.PocketBase
var serverConnections = make(map[string]Server)

func main() {
	app = pocketbase.New()

	// loosely check if it was executed using "go run"
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())

	// enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		// (the isGoRun check is to enable it only during development)
		Automigrate: isGoRun,
	})

	// serve site
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		switch isGoRun {
		case true:
			proxy := httputil.NewSingleHostReverseProxy(&url.URL{
				Scheme: "http",
				Host:   "localhost:5173",
			})
			e.Router.Any("/*", echo.WrapHandler(proxy))
			e.Router.Any("/", echo.WrapHandler(proxy))
		default:
			e.Router.GET("/*", apis.StaticDirectoryHandler(os.DirFS("./site/dist"), true))
		}
		return nil
	})

	// set up cron job to delete records older than 30 days
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		scheduler := cron.New()
		scheduler.MustAdd("delete old records", "* 2 * * *", func() {
			// log.Println("Deleting old records...")
			// Get the current time
			now := time.Now().UTC()
			// Subtract one month
			oneMonthAgo := now.AddDate(0, 0, -30)
			// Format the time as a string
			timeString := oneMonthAgo.Format("2006-01-02 15:04:05")
			// collections to be cleaned
			collections := []string{"system_stats", "container_stats"}

			for _, collection := range collections {
				records, err := app.Dao().FindRecordsByFilter(
					collection,
					fmt.Sprintf("created <= \"%s\"", timeString), // filter
					"", // sort
					-1, // limit
					0,  // offset
				)
				if err != nil {
					log.Println(err)
					return
				}
				// delete records
				for _, record := range records {
					if err := app.Dao().DeleteRecord(record); err != nil {
						log.Fatal(err)
					}
				}
			}
		})
		scheduler.Start()
		return nil
	})

	// ssh key setup
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// create ssh key if it doesn't exist
		getSSHKey()
		// api route to return public key
		e.Router.GET("/api/qoma/getkey", func(c echo.Context) error {
			requestData := apis.RequestInfo(c)
			if requestData.AuthRecord == nil {
				return apis.NewForbiddenError("Forbidden", nil)
			}
			key, err := os.ReadFile("./pb_data/id_ed25519.pub")
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
		e.Router.GET("/api/qoma/first-run", func(c echo.Context) error {
			adminNum, err := app.Dao().TotalAdmins()
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, map[string]bool{"firstRun": adminNum == 0})
		})
		return nil
	})

	// start ticker for server updates
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		go serverUpdateTicker()
		return nil
	})

	// immediately create connection for new servers
	app.OnModelAfterCreate("systems").Add(func(e *core.ModelEvent) error {
		go updateServer(e.Model.(*models.Record))
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

		// if server is set to pending, try to connect
		if newStatus == "pending" {
			go updateServer(newRecord)
		}

		// alerts
		handleStatusAlerts(newStatus, oldRecord)
		return nil
	})

	// do things after a systems record is deleted
	app.OnModelAfterDelete("systems").Add(func(e *core.ModelEvent) error {
		// if server connection exists, close it
		deleteServerConnection(e.Model.(*models.Record))
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func serverUpdateTicker() {
	ticker := time.NewTicker(60 * time.Second)
	for range ticker.C {
		updateServers()
	}
}

func updateServers() {
	// serverCount := len(serverConnections)
	// fmt.Println("server count: ", serverCount)
	query := app.Dao().RecordQuery("systems").
		Where(dbx.NewExp("status != \"paused\"")).
		OrderBy("updated ASC").
		// todo get total count of servers and divide by 4 or something
		Limit(5)

	records := []*models.Record{}
	if err := query.All(&records); err != nil {
		app.Logger().Error("Failed to get servers: ", "err", err.Error())
		// return nil, err
	}

	for _, record := range records {
		updateServer(record)
	}
}

func updateServer(record *models.Record) {
	var server Server
	// check if server connection data exists
	if _, ok := serverConnections[record.Id]; ok {
		server = serverConnections[record.Id]
	} else {
		// create server connection struct
		server = Server{
			Host: record.Get("host").(string),
			Port: record.Get("port").(string),
		}
		client, err := getServerConnection(&server)
		if err != nil {
			app.Logger().Error("Failed to connect:", "err", err.Error(), "server", server.Host, "port", server.Port)
			updateServerStatus(record, "down")
			return
		}
		server.Client = client
		serverConnections[record.Id] = server
	}
	// get server stats from agent
	systemData, err := requestJson(&server)
	if err != nil {
		if err.Error() == "retry" {
			// if previous connection was closed, try again
			app.Logger().Error("Existing SSH connection closed. Retrying...", "host", server.Host, "port", server.Port)
			deleteServerConnection(record)
			updateServer(record)
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
	if err := app.Dao().SaveRecord(system_stats_record); err != nil {
		app.Logger().Error("Failed to save record: ", "err", err.Error())
	}
	// add new container_stats record
	if len(systemData.Containers) > 0 {
		container_stats, _ := app.Dao().FindCollectionByNameOrId("container_stats")
		container_stats_record := models.NewRecord(container_stats)
		container_stats_record.Set("system", record.Id)
		container_stats_record.Set("stats", systemData.Containers)
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

func sendAlert(data EmailData) {
	message := &mailer.Message{
		From: mail.Address{
			Address: app.Settings().Meta.SenderAddress,
			Name:    app.Settings().Meta.SenderName,
		},
		To:      []mail.Address{{Address: data.to}},
		Subject: data.subj,
		Text:    data.body,
	}
	if err := app.NewMailClient().Send(message); err != nil {
		app.Logger().Error("Failed to send alert: ", "err", err.Error())
	}
}

func handleStatusAlerts(newStatus string, oldRecord *models.Record) error {
	var alertStatus string
	switch newStatus {
	case "up":
		if oldRecord.Get("status") == "down" {
			alertStatus = "up"
		}
	case "down":
		if oldRecord.Get("status") == "up" {
			alertStatus = "down"
		}
	}
	if alertStatus == "" {
		return nil
	}
	alerts, err := app.Dao().FindRecordsByFilter("alerts", "name = 'status' && system = {:system}", "-created", -1, 0, dbx.Params{
		"system": oldRecord.Get("id")})
	if err != nil {
		fmt.Println("failed to get users", "err", err.Error())
		return nil
	}
	if len(alerts) == 0 {
		return nil
	}
	// expand the user relation
	if errs := app.Dao().ExpandRecords(alerts, []string{"user"}, nil); len(errs) > 0 {
		return fmt.Errorf("failed to expand: %v", errs)
	}
	systemName := oldRecord.Get("name").(string)
	emoji := "\U0001F534"
	if alertStatus == "up" {
		emoji = "\u2705"
	}
	for _, alert := range alerts {
		user := alert.ExpandedOne("user")
		if user == nil {
			continue
		}
		// send alert
		sendAlert(EmailData{
			to:   user.Get("email").(string),
			subj: fmt.Sprintf("Connection to %s is %s %v", systemName, alertStatus, emoji),
			body: fmt.Sprintf("Connection to %s is %s %v\n\n- Qoma", systemName, alertStatus, emoji),
		})
	}
	return nil
}

func getSSHKey() ([]byte, error) {
	// check if the key pair already exists
	existingKey, err := os.ReadFile("./pb_data/id_ed25519")
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
	privateFile, err := os.Create("./pb_data/id_ed25519")
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
	publicFile, err := os.Create("./pb_data/id_ed25519.pub")
	if err != nil {
		return nil, err
	}
	defer publicFile.Close()

	if _, err := publicFile.Write(pubKeyBytes); err != nil {
		return nil, err
	}

	app.Logger().Info("ed25519 SSH key pair generated successfully.")
	app.Logger().Info("Private key saved to: pb_data/id_ed25519")
	app.Logger().Info("Public key saved to: pb_data/id_ed25519.pub")

	existingKey, err = os.ReadFile("./pb_data/id_ed25519")
	if err == nil {
		return existingKey, nil
	}
	return nil, err
}
