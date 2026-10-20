package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"nut_devices", "nut_stats"} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			collection.Fields.Add(&core.SelectField{
				Name:      "output_power_source",
				Values:    []string{"measured", "estimated", "unknown"},
				MaxSelect: 1,
			})
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, name := range []string{"nut_devices", "nut_stats"} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				return err
			}
			collection.Fields.RemoveByName("output_power_source")
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	})
}
