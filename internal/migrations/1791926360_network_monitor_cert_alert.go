package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const networkMonitorCertAlertName = "NetworkMonitorCert"

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field := collection.Fields.GetByName("name").(*core.SelectField)
		if !slices.Contains(field.Values, networkMonitorCertAlertName) {
			field.Values = append(field.Values, networkMonitorCertAlertName)
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field := collection.Fields.GetByName("name").(*core.SelectField)
		field.Values = slices.DeleteFunc(field.Values, func(name string) bool {
			return name == networkMonitorCertAlertName
		})
		return app.Save(collection)
	})
}
