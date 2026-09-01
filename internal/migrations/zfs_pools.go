package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Creates the zfs_pools collection for per-system ZFS pool detail data
// (pool health, capacity, scrub, vdevs, datasets). Upserts rather than
// deletes missing collections, so it is safe on fresh and existing installs.
func init() {
	m.Register(func(app core.App) error {
		// update collections
		jsonData := `[
	{
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
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "relation1204987316",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text7739291048",
				"max": 0,
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
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text5528164482",
				"max": 0,
				"min": 0,
				"name": "health",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "number8862034195",
				"max": null,
				"min": null,
				"name": "size",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number4418907321",
				"max": null,
				"min": null,
				"name": "alloc",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number2904183765",
				"max": null,
				"min": null,
				"name": "free",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "json4466109723",
				"maxSize": 0,
				"name": "scrub",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "json"
			},
			{
				"hidden": false,
				"id": "json9012873456",
				"maxSize": 0,
				"name": "vdevs",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "json"
			},
			{
				"hidden": false,
				"id": "json7182045639",
				"maxSize": 0,
				"name": "datasets",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "json"
			},
			{
				"hidden": false,
				"id": "date9274163058",
				"max": "",
				"min": "",
				"name": "details_updated",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "date"
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
		"id": "pbc_8441057391",
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_zfsPoolsSystem` + "`" + ` ON ` + "`" + `zfs_pools` + "`" + ` (` + "`" + `system` + "`" + `)"
		],
		"listRule": null,
		"name": "zfs_pools",
		"system": false,
		"type": "base",
		"updateRule": null,
		"viewRule": null
	}
]`

		err := app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
		if err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		return nil
	})
}
