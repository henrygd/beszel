package migrations

import (
	"os"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const (
	TempAdminEmail = "_@b.b"
)

func init() {
	m.Register(func(app core.App) error {
		// initial settings
		settings := app.Settings()
		settings.Meta.AppName = "Beszel"
		settings.Meta.HideControls = true
		settings.Logs.MinLevel = 4
		if err := app.Save(settings); err != nil {
			return err
		}

		// create superuser
		superuserCollection, _ := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		superUser := core.NewRecord(superuserCollection)

		// set email
		email, _ := GetEnv("USER_EMAIL")
		password, _ := GetEnv("USER_PASSWORD")
		didProvideUserDetails := email != "" && password != ""

		// set superuser email
		if email == "" {
			email = TempAdminEmail
		}
		superUser.SetEmail(email)

		// set superuser password
		if password != "" {
			superUser.SetPassword(password)
		} else {
			superUser.SetRandomPassword()
		}

		// if user details are provided, we create a regular user as well
		if didProvideUserDetails {
			usersCollection, _ := app.FindCollectionByNameOrId("users")
			user := core.NewRecord(usersCollection)
			user.SetEmail(email)
			user.SetPassword(password)
			err := app.Save(user)
			if err != nil {
				return err
			}
		}

		return app.Save(superUser)
	}, nil)
}

// GetEnv retrieves an environment variable with a "BESZEL_HUB_" prefix, or falls back to the unprefixed key.
func GetEnv(key string) (value string, exists bool) {
	if value, exists = os.LookupEnv("BESZEL_HUB_" + key); exists {
		return value, exists
	}
	// Fallback to the old unprefixed key
	return os.LookupEnv(key)
}
