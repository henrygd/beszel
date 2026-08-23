package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
			{
				"id": "hubsettings12345",
				"listRule": "@request.auth.id != \"\"",
				"viewRule": "@request.auth.id != \"\"",
				"createRule": "@request.auth.role = 'admin'",
				"updateRule": "@request.auth.role = 'admin'",
				"deleteRule": "@request.auth.role = 'admin'",
				"name": "hub_settings",
				"type": "base",
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
						"id": "retention000001",
						"maxSelect": 1,
						"name": "retention",
						"presentable": false,
						"required": true,
						"system": false,
						"type": "select",
						"values": ["30d","60d","90d","180d","365d","730d","1095d","1825d","never"]
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
				"indexes": [],
				"system": false
			}
		]`
		if err := app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false); err != nil {
			return err
		}
		// create default singleton record if not exists
		count, err := app.CountRecords("hub_settings")
		if err != nil {
			return err
		}
		if count == 0 {
			collection, err := app.FindCollectionByNameOrId("hub_settings")
			if err != nil {
				return err
			}
			record := core.NewRecord(collection)
			record.Set("id", "hubsettings0001")
			record.Set("retention", "30d")
			if err := app.Save(record); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("hub_settings")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
