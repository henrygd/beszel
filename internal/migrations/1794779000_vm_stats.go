package migrations

import (
	"encoding/json"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `{
			"id": "vmstatshistory1",
			"listRule": null,
			"viewRule": null,
			"createRule": null,
			"updateRule": null,
			"deleteRule": null,
			"name": "vm_stats",
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
					"cascadeDelete": true,
					"collectionId": "2hz5ncl8tizk5nx",
					"hidden": false,
					"id": "vmstatsysrel01",
					"maxSelect": 1,
					"minSelect": 0,
					"name": "system",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "relation"
				},
				{
					"hidden": false,
					"id": "vmstatsjson001",
					"maxSize": 2000000,
					"name": "stats",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "json"
				},
				{
					"hidden": false,
					"id": "vmstatstype001",
					"maxSelect": 1,
					"name": "type",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "select",
					"values": ["1m", "10m", "20m", "120m", "480m"]
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
				"CREATE INDEX ` + "`" + `idx_vm_stats_system_type_created` + "`" + ` ON ` + "`" + `vm_stats` + "`" + ` (` + "`" + `system` + "`" + `, ` + "`" + `type` + "`" + `, ` + "`" + `created` + "`" + `)"
			],
			"system": false
		}`
		collection := &core.Collection{}
		if err := json.Unmarshal([]byte(jsonData), collection); err != nil {
			return err
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("vmstatshistory1")
		if err != nil {
			return err
		}
		return app.Delete(collection)
	})
}
