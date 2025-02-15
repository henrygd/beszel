package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_132971995")
		if err != nil {
			return err
		}

		// update field
		if err := collection.Fields.AddMarshaledJSONAt(1, []byte(`{
			"hidden": false,
			"id": "select3221973358",
			"maxSelect": 1,
			"name": "withAPIKey",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "select",
			"values": [
				"accept",
				"deny",
				"display"
			]
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_132971995")
		if err != nil {
			return err
		}

		// update field
		if err := collection.Fields.AddMarshaledJSONAt(1, []byte(`{
			"hidden": false,
			"id": "select3221973358",
			"maxSelect": 1,
			"name": "withAPIKey",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "select",
			"values": [
				"accept",
				"deny",
				"block",
				"display"
			]
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	})
}
