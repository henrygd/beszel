package migrations

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// 1. Create container_alerts collection
		// Check if collection already exists to prevent errors on restart
		if _, err := app.FindCollectionByNameOrId("container_alerts"); err == nil {
			// Collection exists, skip creation
		} else {
			jsonData := `{
			"id": "ca_container_alerts",
			"name": "container_alerts",
			"type": "base",
			"system": false,
			"listRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
			"viewRule": "",
			"createRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
			"updateRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
			"deleteRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
			"fields": [
				{
					"id": "text_id",
					"name": "id",
					"type": "text",
					"required": true,
					"presentable": false,
					"unique": false,
					"system": true,
					"primaryKey": true,
					"autogeneratePattern": "[a-z0-9]{15}",
					"min": 15,
					"max": 15,
					"pattern": "^[a-z0-9]+$"
				},
				{
					"id": "rel_user",
					"name": "user",
					"type": "relation",
					"required": true,
					"presentable": false,
					"unique": false,
					"system": false,
					"collectionId": "_pb_users_auth_",
					"cascadeDelete": true,
					"minSelect": null,
					"maxSelect": 1,
					"displayFields": null
				},
				{
					"id": "rel_system",
					"name": "system",
					"type": "relation",
					"required": true,
					"presentable": false,
					"unique": false,
					"system": false,
					"collectionId": "2hz5ncl8tizk5nx",
					"cascadeDelete": true,
					"minSelect": null,
					"maxSelect": 1,
					"displayFields": null
				},
				{
					"id": "text_container",
					"name": "container",
					"type": "text",
					"required": true,
					"presentable": false,
					"unique": false,
					"system": false,
					"min": null,
					"max": null,
					"pattern": ""
				},
				{
					"id": "select_name",
					"name": "name",
					"type": "select",
					"required": true,
					"presentable": false,
					"unique": false,
					"system": false,
					"maxSelect": 1,
					"values": [
						"Status",
						"CPU",
						"Memory",
						"Network",
						"Health"
					]
				},
				{
					"id": "number_value",
					"name": "value",
					"type": "number",
					"required": false,
					"presentable": false,
					"unique": false,
					"system": false,
					"min": null,
					"max": null,
					"onlyInt": false
				},
				{
					"id": "number_min",
					"name": "min",
					"type": "number",
					"required": false,
					"presentable": false,
					"unique": false,
					"system": false,
					"min": null,
					"max": 60,
					"onlyInt": true
				},
				{
					"id": "bool_triggered",
					"name": "triggered",
					"type": "bool",
					"required": false,
					"presentable": false,
					"unique": false,
					"system": false
				},
				{
					"id": "autodate_created",
					"name": "created",
					"type": "autodate",
					"required": false,
					"presentable": false,
					"unique": false,
					"system": false,
					"onCreate": true,
					"onUpdate": false
				},
				{
					"id": "autodate_updated",
					"name": "updated",
					"type": "autodate",
					"required": false,
					"presentable": false,
					"unique": false,
					"system": false,
					"onCreate": true,
					"onUpdate": true
				}
			],
			"indexes": [
				"CREATE UNIQUE INDEX ` + "`" + `idx_container_alerts_unique` + "`" + ` ON ` + "`" + `container_alerts` + "`" + ` (` + "`" + `user` + "`" + `, ` + "`" + `system` + "`" + `, ` + "`" + `container` + "`" + `, ` + "`" + `name` + "`" + `)"
			]
		}`

			collection := &core.Collection{}
			if err := json.Unmarshal([]byte(jsonData), collection); err != nil {
				return err
			}

			if err := app.Save(collection); err != nil {
				return err
			}
		}

		// 2. Add container field to alerts_history collection
		alertsHistoryCollection, err := app.FindCollectionByNameOrId("alerts_history")
		if err != nil {
			return err
		}

		if alertsHistoryCollection.Fields.GetByName("container") == nil {
			containerField := &core.TextField{
				Name:     "container",
				Required: false,
			}

			alertsHistoryCollection.Fields.Add(containerField)

			return app.Save(alertsHistoryCollection)
		}

		return nil
	}, func(app core.App) error {
		// Rollback 2: remove the container field
		alertsHistoryCollection, err := app.FindCollectionByNameOrId("alerts_history")
		if err == nil {
			if field := alertsHistoryCollection.Fields.GetByName("container"); field != nil {
				alertsHistoryCollection.Fields.RemoveByName("container")
				if err := app.Save(alertsHistoryCollection); err != nil {
					return err
				}
			}
		}

		// Rollback 1: delete the container_alerts collection
		collection, err := app.FindCollectionByNameOrId("container_alerts")
		if err == nil {
			return app.Delete(collection)
		}

		return nil
	})
}
