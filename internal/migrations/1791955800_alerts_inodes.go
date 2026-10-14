package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		nameField, ok := collection.Fields.GetByName("name").(*core.SelectField)
		if !ok {
			return nil
		}
		if !slices.Contains(nameField.Values, "Inodes") {
			nameField.Values = append(nameField.Values, "Inodes")
			return app.Save(collection)
		}
		return nil
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		nameField, ok := collection.Fields.GetByName("name").(*core.SelectField)
		if !ok {
			return nil
		}
		nameField.Values = slices.DeleteFunc(nameField.Values, func(v string) bool {
			return v == "Inodes"
		})
		return app.Save(collection)
	})
}
