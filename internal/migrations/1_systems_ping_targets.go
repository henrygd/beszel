package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("ping_targets") != nil {
			return nil
		}
		collection.Fields.Add(&core.TextField{
			Name: "ping_targets",
			Id:   "ping_targets_field",
		})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("ping_targets")
		return app.Save(collection)
	})
}
