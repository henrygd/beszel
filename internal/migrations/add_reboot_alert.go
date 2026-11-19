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
		nameField := collection.Fields.GetByName("name")
		if nameField == nil {
			return nil
		}
		selectField, ok := nameField.(*core.SelectField)
		if !ok {
			return nil
		}
		for _, v := range selectField.Values {
			if v == "Reboot" {
				return nil
			}
		}
		selectField.Values = append(selectField.Values, "Reboot")
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		nameField := collection.Fields.GetByName("name")
		if nameField == nil {
			return nil
		}
		selectField, ok := nameField.(*core.SelectField)
		if !ok {
			return nil
		}
		filtered := selectField.Values[:0]
		for _, v := range selectField.Values {
			if v != "Reboot" {
				filtered = append(filtered, v)
			}
		}
		selectField.Values = filtered
		return app.Save(collection)
	})
}
