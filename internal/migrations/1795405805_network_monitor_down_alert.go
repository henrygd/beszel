package migrations

import (
	"slices"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

const networkMonitorDownAlertName = "NetworkMonitorDown"

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field := collection.Fields.GetByName("name").(*core.SelectField)
		if !slices.Contains(field.Values, networkMonitorDownAlertName) {
			field.Values = append(field.Values, networkMonitorDownAlertName)
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field := collection.Fields.GetByName("name").(*core.SelectField)
		field.Values = slices.DeleteFunc(field.Values, func(name string) bool {
			return name == networkMonitorDownAlertName
		})
		return app.Save(collection)
	})
}
