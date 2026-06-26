package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds container health alerts support:
//   - "ContainerHealth" option on the alerts "name" select field
//   - "container" text field on alerts and alerts_history (empty = any container)
//   - alerts unique index extended with "container" so multiple ContainerHealth
//     alerts (one per container + one "any") can coexist for a system
func init() {
	const alertName = "ContainerHealth"
	const oldIndex = "CREATE UNIQUE INDEX `idx_MnhEt21L5r` ON `alerts` (`user`, `system`, `name`)"
	const newIndex = "CREATE UNIQUE INDEX `idx_MnhEt21L5r` ON `alerts` (`user`, `system`, `name`, `container`)"

	m.Register(func(app core.App) error {
		alerts, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		if field, ok := alerts.Fields.GetByName("name").(*core.SelectField); ok {
			if !slices.Contains(field.Values, alertName) {
				field.Values = append(field.Values, alertName)
			}
		}
		if alerts.Fields.GetByName("container") == nil {
			alerts.Fields.Add(&core.TextField{Name: "container"})
		}
		// extend the unique index to include container
		for i, idx := range alerts.Indexes {
			if normalizeIndex(idx) == normalizeIndex(oldIndex) {
				alerts.Indexes[i] = newIndex
			}
		}
		if err := app.Save(alerts); err != nil {
			return err
		}

		history, err := app.FindCollectionByNameOrId("alerts_history")
		if err != nil {
			return err
		}
		if history.Fields.GetByName("container") == nil {
			history.Fields.Add(&core.TextField{Name: "container"})
		}
		return app.Save(history)
	}, func(app core.App) error {
		alerts, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		if field, ok := alerts.Fields.GetByName("name").(*core.SelectField); ok {
			field.Values = slices.DeleteFunc(field.Values, func(v string) bool {
				return v == alertName
			})
		}
		alerts.Fields.RemoveByName("container")
		for i, idx := range alerts.Indexes {
			if normalizeIndex(idx) == normalizeIndex(newIndex) {
				alerts.Indexes[i] = oldIndex
			}
		}
		if err := app.Save(alerts); err != nil {
			return err
		}

		history, err := app.FindCollectionByNameOrId("alerts_history")
		if err != nil {
			return err
		}
		history.Fields.RemoveByName("container")
		return app.Save(history)
	})
}

// normalizeIndex strips whitespace so index definitions can be compared
// regardless of formatting differences (newlines/spaces).
func normalizeIndex(idx string) string {
	var b []byte
	for i := 0; i < len(idx); i++ {
		c := idx[i]
		if c == ' ' || c == '\n' || c == '\t' {
			continue
		}
		b = append(b, c)
	}
	return string(b)
}
