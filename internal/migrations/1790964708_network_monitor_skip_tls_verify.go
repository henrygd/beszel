package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.BoolField{Id: "nm_skip_tls_verify", Name: "skipTlsVerify"})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("skipTlsVerify")
		return app.Save(collection)
	})
}
