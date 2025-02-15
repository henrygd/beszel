package migrations

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `{
			"createRule": null,
			"deleteRule": null,
			"fields": [
				{
					"autogeneratePattern": "[a-z0-9]{15}",
					"hidden": false,
					"id": "text3208210256",
					"max": 15,
					"min": 15,
					"name": "id",
					"pattern": "^[a-z0-9]+$",
					"presentable": false,
					"primaryKey": true,
					"required": true,
					"system": true,
					"type": "text"
				},
				{
					"hidden": false,
					"id": "select3642302309",
					"maxSelect": 1,
					"name": "withoutAPIKey",
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
				},
				{
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
				},
				{
					"hidden": false,
					"id": "autodate2990389176",
					"name": "created",
					"onCreate": true,
					"onUpdate": false,
					"presentable": false,
					"system": false,
					"type": "autodate"
				},
				{
					"hidden": false,
					"id": "autodate3332085495",
					"name": "updated",
					"onCreate": true,
					"onUpdate": true,
					"presentable": false,
					"system": false,
					"type": "autodate"
				}
			],
			"id": "pbc_132971995",
			"indexes": [],
			"listRule": null,
			"name": "connection_settings",
			"system": false,
			"type": "base",
			"updateRule": null,
			"viewRule": null
		}`

		collection := &core.Collection{}
		if err := json.Unmarshal([]byte(jsonData), &collection); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_132971995")
		if err != nil {
			return err
		}

		return app.Delete(collection)
	})
}
