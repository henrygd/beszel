package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// networkMonitorDownLatencyAlertNames are the alert names added by this migration.
var networkMonitorDownLatencyAlertNames = []string{"NetworkMonitorDown", "NetworkMonitorLatency"}

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field := collection.Fields.GetByName("name").(*core.SelectField)
		for _, name := range networkMonitorDownLatencyAlertNames {
			if !slices.Contains(field.Values, name) {
				field.Values = append(field.Values, name)
			}
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field := collection.Fields.GetByName("name").(*core.SelectField)
		field.Values = slices.DeleteFunc(field.Values, func(name string) bool {
			return slices.Contains(networkMonitorDownLatencyAlertNames, name)
		})
		return app.Save(collection)
	})
}
