package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const packageUpdatesAlertName = "PackageUpdates"

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}

		nameField := collection.Fields.GetByName("name").(*core.SelectField)
		if !slices.Contains(nameField.Values, packageUpdatesAlertName) {
			nameField.Values = append(nameField.Values, packageUpdatesAlertName)
		}

		minField := collection.Fields.GetByName("min").(*core.NumberField)
		min := float64(0)
		max := float64(43200)
		minField.Min = &min
		minField.Max = &max
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}

		nameField := collection.Fields.GetByName("name").(*core.SelectField)
		nameField.Values = slices.DeleteFunc(nameField.Values, func(name string) bool {
			return name == packageUpdatesAlertName
		})

		minField := collection.Fields.GetByName("min").(*core.NumberField)
		minField.Min = nil
		max := float64(60)
		minField.Max = &max
		return app.Save(collection)
	})
}
