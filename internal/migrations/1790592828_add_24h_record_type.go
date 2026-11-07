package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	collections := []string{"system_stats", "container_stats", "network_monitor_stats"}
	m.Register(func(app core.App) error {
		for _, name := range collections {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			typeField := collection.Fields.GetByName("type")
			if selectField, ok := typeField.(*core.SelectField); ok {
				if !slices.Contains(selectField.Values, "24h") {
					selectField.Values = append(selectField.Values, "24h")
				}
			}
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range collections {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			typeField := collection.Fields.GetByName("type")
			if selectField, ok := typeField.(*core.SelectField); ok {
				newValues := make([]string, 0, len(selectField.Values))
				for _, v := range selectField.Values {
					if v != "24h" {
						newValues = append(newValues, v)
					}
				}
				selectField.Values = newValues
			}
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	})
}
