package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("smart_devices")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("smart_history") == nil {
			collection.Fields.Add(&core.JSONField{Name: "smart_history", MaxSize: 2_000_000, Hidden: true})
		}
		if collection.Fields.GetByName("smart_health") == nil {
			collection.Fields.Add(&core.JSONField{Name: "smart_health", MaxSize: 100_000})
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("smart_devices")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("smart_history")
		collection.Fields.RemoveByName("smart_health")
		return app.Save(collection)
	})
}
