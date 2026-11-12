package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.Fields.Add(
			&core.TextField{Id: "nm_expected_status", Name: "expectedStatus", Max: 100},
			&core.TextField{Id: "nm_keyword", Name: "keyword", Max: 200},
			&core.BoolField{Id: "nm_invert_keyword", Name: "invertKeyword"},
		)
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("expectedStatus")
		collection.Fields.RemoveByName("keyword")
		collection.Fields.RemoveByName("invertKeyword")
		return app.Save(collection)
	})
}
