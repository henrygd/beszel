// Package users handles user-related custom functionality.
package users

import (
	"beszel/migrations"
	"log"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

type UserManager struct {
	app *pocketbase.PocketBase
}

type UserSettings struct {
	ChartTime            string   `json:"chartTime"`
	NotificationEmails   []string `json:"emails"`
	NotificationWebhooks []string `json:"webhooks"`
	// Language             string   `json:"lang"`
}

func NewUserManager(app *pocketbase.PocketBase) *UserManager {
	return &UserManager{
		app: app,
	}
}

// Initialize user role if not set
func (um *UserManager) InitializeUserRole(e *core.RecordEvent) error {
	if e.Record.GetString("role") == "" {
		e.Record.Set("role", "user")
	}
	return e.Next()
}

// Initialize user settings with defaults if not set
func (um *UserManager) InitializeUserSettings(e *core.RecordEvent) error {
	record := e.Record
	// intialize settings with defaults
	settings := UserSettings{
		// Language:             "en",
		ChartTime:            "1h",
		NotificationEmails:   []string{},
		NotificationWebhooks: []string{},
	}
	record.UnmarshalJSONField("settings", &settings)
	if len(settings.NotificationEmails) == 0 {
		// get user email from auth record
		if errs := um.app.ExpandRecord(record, []string{"user"}, nil); len(errs) == 0 {
			// app.Logger().Error("failed to expand user relation", "errs", errs)
			if user := record.ExpandedOne("user"); user != nil {
				settings.NotificationEmails = []string{user.GetString("email")}
			} else {
				log.Println("Failed to get user email from auth record")
			}
		} else {
			log.Println("failed to expand user relation", "errs", errs)
		}
	}
	// if len(settings.NotificationWebhooks) == 0 {
	// 	settings.NotificationWebhooks = []string{""}
	// }
	record.Set("settings", settings)
	return e.Next()
}

// Custom API endpoint to create the first user.
// Mimics previous default behavior in PocketBase < 0.23.0 allowing user to be created through the Beszel UI.
func (um *UserManager) CreateFirstUser(e *core.RequestEvent) error {
	// check that there are no users
	totalUsers, err := um.app.CountRecords("users")
	if err != nil || totalUsers > 0 {
		return e.JSON(http.StatusForbidden, map[string]string{"err": "Forbidden"})
	}
	// check that there is only one superuser and the email matches the email of the superuser we set up in initial-settings.go
	adminUsers, err := um.app.FindAllRecords(core.CollectionNameSuperusers)
	if err != nil || len(adminUsers) != 1 || adminUsers[0].GetString("email") != migrations.TempAdminEmail {
		return e.JSON(http.StatusForbidden, map[string]string{"err": "Forbidden"})
	}
	// create first user using supplied email and password in request body
	data := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{}
	if err := e.BindBody(&data); err != nil {
		return e.JSON(http.StatusBadRequest, map[string]string{"err": err.Error()})
	}
	if data.Email == "" || data.Password == "" {
		return e.JSON(http.StatusBadRequest, map[string]string{"err": "Bad request"})
	}

	collection, _ := um.app.FindCollectionByNameOrId("users")
	user := core.NewRecord(collection)
	user.SetEmail(data.Email)
	user.SetPassword(data.Password)
	user.Set("role", "admin")
	user.Set("verified", true)
	if username := strings.Split(data.Email, "@")[0]; len(username) > 2 {
		user.Set("username", username)
	}
	if err := um.app.Save(user); err != nil {
		return e.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
	}
	// create superuser using the email of the first user
	collection, _ = um.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	adminUser := core.NewRecord(collection)
	adminUser.SetEmail(data.Email)
	adminUser.SetPassword(data.Password)
	if err := um.app.Save(adminUser); err != nil {
		return e.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
	}
	// delete the intial superuser
	if err := um.app.Delete(adminUsers[0]); err != nil {
		return e.JSON(http.StatusInternalServerError, map[string]string{"err": err.Error()})
	}
	return e.JSON(http.StatusOK, map[string]string{"msg": "User created"})
}
