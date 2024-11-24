// Package users handles user-related custom functionality.
package users

import (
	"log"

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

// todo: test
func (um *UserManager) InitializeUserRole(e *core.RecordEvent) error {
	if e.Record.GetString("role") == "" {
		e.Record.Set("role", "user")
	}
	return e.Next()
}

// todo: test
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
