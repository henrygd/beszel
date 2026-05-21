package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
	{
		"id": "np_probes_001",
		"listRule": null,
		"viewRule": null,
		"createRule": null,
		"updateRule": null,
		"deleteRule": null,
		"name": "network_probes",
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
				"id": "np_system",
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
				"id": "np_name",
				"max": 200,
				"min": 0,
				"name": "name",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "np_target",
				"max": 500,
				"min": 1,
				"name": "target",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": true,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "np_protocol",
				"maxSelect": 1,
				"name": "protocol",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "select",
				"values": ["icmp", "tcp", "http"]
			},
			{
				"hidden": false,
				"id": "np_port",
				"max": 65535,
				"min": 0,
				"name": "port",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "np_interval",
				"max": 3600,
				"min": 1,
				"name": "interval",
				"onlyInt": true,
				"presentable": false,
				"required": true,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "np_enabled",
				"name": "enabled",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "bool"
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
			"CREATE INDEX ` + "`" + `idx_np_system_enabled` + "`" + ` ON ` + "`" + `network_probes` + "`" + ` (\n  ` + "`" + `system` + "`" + `,\n  ` + "`" + `enabled` + "`" + `\n)"
		],
		"system": false
	},
	{
		"id": "np_stats_001",
		"listRule": null,
		"viewRule": null,
		"createRule": null,
		"updateRule": null,
		"deleteRule": null,
		"name": "network_probe_stats",
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
				"id": "nps_system",
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
				"id": "nps_stats",
				"maxSize": 2000000,
				"name": "stats",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "json"
			},
			{
				"hidden": false,
				"id": "nps_type",
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
			"CREATE INDEX ` + "`" + `idx_nps_system_type_created` + "`" + ` ON ` + "`" + `network_probe_stats` + "`" + ` (\n  ` + "`" + `system` + "`" + `,\n  ` + "`" + `type` + "`" + `,\n  ` + "`" + `created` + "`" + `\n)"
		],
		"system": false
	}
]`

		return app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
	}, func(app core.App) error {
		// down: remove the network probe collections
		if c, err := app.FindCollectionByNameOrId("network_probes"); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		if c, err := app.FindCollectionByNameOrId("network_probe_stats"); err == nil {
			if err := app.Delete(c); err != nil {
				return err
			}
		}
		return nil
	})
}
