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

		// Add tags field (JSON array for dynamic string values)
		collection.Fields.Add(&core.JSONField{
			Id:          "tags1234567890",
			Name:        "tags",
			Required:    false,
			Presentable: false,
			MaxSize:     2000000,
		})

		// Add group field (text)
		collection.Fields.Add(&core.TextField{
			Id:         "group1234567890",
			Name:       "group",
			Required:   false,
			Presentable: false,
			Max:        0,
			Min:        0,
			Pattern:    "",
		})

		return app.Save(collection)
	}, func(app core.App) error {
		// Down migration - remove the fields
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}

		collection.Fields.RemoveById("tags1234567890")
		collection.Fields.RemoveById("group1234567890")

		return app.Save(collection)
	})
}
