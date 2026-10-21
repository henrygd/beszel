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
		collection.Fields.Add(&core.TextField{Id: "nm_name", Name: "name", Max: 100})
		// Names are unique per system, so the same target can share a name across systems.
		collection.AddIndex("idx_nm_system_name", true, "`system`, `name`", "`name` != ''")
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.RemoveIndex("idx_nm_system_name")
		collection.Fields.RemoveByName("name")
		return app.Save(collection)
	})
}
