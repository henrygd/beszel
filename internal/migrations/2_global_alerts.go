package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Skip if already created by snapshot migration (fresh install)
		if _, err := app.FindCollectionByNameOrId("global_alerts"); err == nil {
			return nil
		}

		jsonData := `{
			"id": "global_alerts_coll1",
			"listRule": "@request.auth.id != \"\"",
			"viewRule": "@request.auth.id != \"\"",
			"createRule": "@request.auth.superuser = true || @request.auth.role = 'admin'",
			"updateRule": "@request.auth.superuser = true || @request.auth.role = 'admin'",
			"deleteRule": "@request.auth.superuser = true || @request.auth.role = 'admin'",
			"name": "global_alerts",
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
					"id": "ga_name",
					"maxSelect": 1,
					"name": "name",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "select",
					"values": [
						"Status",
						"CPU",
						"Memory",
						"Disk",
						"Temperature",
						"Bandwidth",
						"GPU",
						"LoadAvg1",
						"LoadAvg5",
						"LoadAvg15",
						"Battery"
					]
				},
				{
					"hidden": false,
					"id": "ga_value",
					"max": null,
					"min": null,
					"name": "value",
					"onlyInt": false,
					"presentable": false,
					"required": false,
					"system": false,
					"type": "number"
				},
				{
					"hidden": false,
					"id": "ga_min",
					"max": 60,
					"min": null,
					"name": "min",
					"onlyInt": true,
					"presentable": false,
					"required": false,
					"system": false,
					"type": "number"
				},
				{
					"hidden": false,
					"id": "ga_excluded_systems",
					"maxSize": 0,
					"name": "excluded_systems",
					"presentable": false,
					"required": false,
					"system": false,
					"type": "json"
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
			"indexes": [
				"CREATE UNIQUE INDEX ` + "`" + `idx_global_alerts_name` + "`" + ` ON ` + "`" + `global_alerts` + "`" + ` (` + "`" + `name` + "`" + `)"
			],
			"system": false
		}`

		collection := &core.Collection{}
		if err := collection.UnmarshalJSON([]byte(jsonData)); err != nil {
			return err
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("global_alerts")
		if err != nil {
			return nil // already gone
		}
		return app.Delete(collection)
	})
}
