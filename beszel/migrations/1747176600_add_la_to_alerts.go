package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}

		collection.Fields.AddAt(1, &core.SelectField{
			Hidden:      false,
			MaxSelect:   1,
			Name:        "name",
			Presentable: false,
			Required:    false,
			Values: []string{
				"Status",
				"CPU",
				"Memory",
				"Disk",
				"Temperature",
				"Bandwidth",
				"LoadAvg1",  // addedd
				"LoadAvg5",  // addedd
				"LoadAvg15", // addedd
			},
		})

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}

		collection.Fields.AddAt(1, &core.SelectField{
			Hidden:      false,
			MaxSelect:   1,
			Name:        "name",
			Presentable: false,
			Required:    false,
			Values: []string{
				"Status",
				"CPU",
				"Memory",
				"Disk",
				"Temperature",
				"Bandwidth",
			},
		})

		return app.Save(collection)
	})
}
