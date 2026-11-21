package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// JSON definition of mobile_devices collection
		jsonData := `[
			{
				"id": "pbc_mobdevs00001",
				"name": "mobile_devices",
				"type": "base",
				"system": false,
				"listRule": "@request.auth.id != \"\" && user = @request.auth.id",
				"viewRule": "@request.auth.id != \"\" && user = @request.auth.id",
				"createRule": "@request.auth.id != \"\" && user = @request.auth.id",
				"updateRule": "@request.auth.id != \"\" && user = @request.auth.id",
				"deleteRule": "@request.auth.id != \"\" && user = @request.auth.id",
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
						"collectionId": "_pb_users_auth_",
						"hidden": false,
						"id": "mobdevuser01",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "user",
						"presentable": false,
						"required": true,
						"system": false,
						"type": "relation"
					},
					{
						"hidden": false,
						"id": "mobdevtoken01",
						"max": null,
						"min": null,
						"name": "token",
						"presentable": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "mobdevid0001",
						"max": null,
						"min": null,
						"name": "device_id",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "text"
					}
				],
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_mobile_devices_user_device` + "`" + ` ON ` + "`" + `mobile_devices` + "`" + ` (` + "`" + `user` + "`" + `, ` + "`" + `device_id` + "`" + `)"
				]
			}
		]`

		// import collections, set deleteMissing to false to keep all other collections
		err := app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
		if err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("mobile_devices")
		if err != nil {
			return nil // not found, nothing to do
		}
		return app.Delete(collection)
	})
}
