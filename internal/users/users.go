// Package users handles user-related custom functionality.
package users

import (
	"errors"
	"log"
	"net/http"

	"github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type UserManager struct {
	app core.App
}

var errFirstUserForbidden = errors.New("first user setup is not available")

func NewUserManager(app core.App) *UserManager {
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
	// intialize settings with defaults (zero values can be ignored)
	settings := struct {
		ChartTime string   `json:"chartTime"`
		Emails    []string `json:"emails"`
	}{
		ChartTime: "1h",
	}
	record.UnmarshalJSONField("settings", &settings)
	// get user email from auth record
	var user struct {
		Email string `db:"email"`
	}
	err := e.App.DB().NewQuery("SELECT email FROM users WHERE id = {:id}").Bind(dbx.Params{
		"id": record.GetString("user"),
	}).One(&user)
	if err != nil {
		log.Println("failed to get user email", "err", err)
		return err
	}
	settings.Emails = []string{user.Email}
	record.Set("settings", settings)
	return e.Next()
}

// Custom API endpoint to create the first user.
// Mimics previous default behavior in PocketBase < 0.23.0 allowing user to be created through the Beszel UI.
func (um *UserManager) CreateFirstUser(e *core.RequestEvent) error {
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

	err := um.app.RunInTransaction(func(txApp core.App) error {
		// Re-check inside the transaction to prevent concurrent setup requests.
		totalUsers, err := txApp.CountRecords("users")
		if err != nil {
			return err
		}
		adminUsers, err := txApp.FindAllRecords(core.CollectionNameSuperusers)
		if err != nil {
			return err
		}
		if totalUsers > 0 || len(adminUsers) != 1 || adminUsers[0].GetString("email") != migrations.TempAdminEmail {
			return errFirstUserForbidden
		}

		collection, err := txApp.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		user := core.NewRecord(collection)
		user.SetEmail(data.Email)
		user.SetPassword(data.Password)
		user.Set("role", "admin")
		user.Set("verified", true)
		if err := txApp.Save(user); err != nil {
			return err
		}

		collection, err = txApp.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if err != nil {
			return err
		}
		adminUser := core.NewRecord(collection)
		adminUser.SetEmail(data.Email)
		adminUser.SetPassword(data.Password)
		if err := txApp.Save(adminUser); err != nil {
			return err
		}
		return txApp.Delete(adminUsers[0])
	})
	if errors.Is(err, errFirstUserForbidden) {
		return e.JSON(http.StatusForbidden, map[string]string{"err": "Forbidden"})
	}
	if err != nil {
		return e.JSON(http.StatusInternalServerError, map[string]string{"err": "Unable to create user"})
	}
	return e.JSON(http.StatusOK, map[string]string{"msg": "User created"})
}
