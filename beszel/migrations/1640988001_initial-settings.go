package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

var (
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
		collection, _ := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		user := core.NewRecord(collection)
		user.SetEmail(TempAdminEmail)
		user.SetRandomPassword()
		return app.Save(user)
	}, nil)
}
