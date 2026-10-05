package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("fingerprints")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.TextField{
			Name:                "host_key",
			Max:                 100,
			AutogeneratePattern: "",
		})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("fingerprints")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("host_key")
		return app.Save(collection)
	})
}
