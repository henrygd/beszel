package migrations

import (
	"errors"
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// alertNameSystemdFailed is the alerts.name select value for the
// "failed systemd services" alert type.
const alertNameSystemdFailed = "SystemdFailed"

func init() {
	m.Register(func(app core.App) error {
		return updateAlertNameValues(app, func(values []string) []string {
			if slices.Contains(values, alertNameSystemdFailed) {
				return values
			}
			return append(values, alertNameSystemdFailed)
		})
	}, func(app core.App) error {
		return updateAlertNameValues(app, func(values []string) []string {
			return slices.DeleteFunc(values, func(value string) bool {
				return value == alertNameSystemdFailed
			})
		})
	})
}

// updateAlertNameValues applies fn to the values of the alerts.name select field and saves the collection.
func updateAlertNameValues(app core.App, fn func([]string) []string) error {
	collection, err := app.FindCollectionByNameOrId("alerts")
	if err != nil {
		return err
	}
	field, ok := collection.Fields.GetByName("name").(*core.SelectField)
	if !ok {
		return errors.New("alerts.name is not a select field")
	}
	field.Values = fn(field.Values)
	return app.Save(collection)
}
