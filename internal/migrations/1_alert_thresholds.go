package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("thresholds") != nil {
			return nil
		}
		collection.Fields.Add(&core.JSONField{
			Name:    "thresholds",
			MaxSize: 10_000,
		})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("thresholds") == nil {
			return nil
		}
		collection.Fields.RemoveByName("thresholds")
		return app.Save(collection)
	})
}
