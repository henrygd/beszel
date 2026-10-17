package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("system_details")
		if err != nil {
			return err
		}
		// skip if field already exists
		if collection.Fields.GetByName("cpu_mhz") != nil {
			return nil
		}
		collection.Fields.Add(&core.NumberField{
			Name: "cpu_mhz",
		})
		return app.Save(collection)
	}, nil)
}
