package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// 1_add_fanspeed_alert: extends the `alerts.name` select field with the new
// "FanSpeed" option introduced alongside fan RPM monitoring. Existing installs
// already have the alerts collection from the initial snapshot, so we mutate
// it in place rather than restating the whole schema.
func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field, _ := collection.Fields.GetByName("name").(*core.SelectField)
		if field == nil {
			return fmt.Errorf("alerts.name field not found or not a select")
		}
		for _, v := range field.Values {
			if v == "FanSpeed" {
				return nil // idempotent — already applied
			}
		}
		// Slot FanSpeed next to Temperature to mirror their semantic pairing
		// in the UI and to match the order used by the FanChart sitting next
		// to the Temperature chart on the system page.
		newValues := make([]string, 0, len(field.Values)+1)
		for _, v := range field.Values {
			newValues = append(newValues, v)
			if v == "Temperature" {
				newValues = append(newValues, "FanSpeed")
			}
		}
		field.Values = newValues
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field, _ := collection.Fields.GetByName("name").(*core.SelectField)
		if field == nil {
			return nil
		}
		filtered := make([]string, 0, len(field.Values))
		for _, v := range field.Values {
			if v != "FanSpeed" {
				filtered = append(filtered, v)
			}
		}
		field.Values = filtered
		return app.Save(collection)
	})
}
