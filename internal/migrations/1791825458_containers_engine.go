package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("containers")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.TextField{Id: "containers_engine", Name: "engine", Max: 260})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("containers")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("engine")
		return app.Save(collection)
	})
}
