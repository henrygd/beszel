package migrations

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_1243588648")
		if err != nil {
			return err
		}

		// update collection data
		if err := json.Unmarshal([]byte(`{
			"createRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
			"deleteRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
			"listRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id",
			"updateRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
			"viewRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id"
		}`), &collection); err != nil {
			return err
		}

		// add field
		if err := collection.Fields.AddMarshaledJSONAt(4, []byte(`{
			"cascadeDelete": false,
			"collectionId": "_pb_users_auth_",
			"hidden": false,
			"id": "relation344172009",
			"maxSelect": 999,
			"minSelect": 0,
			"name": "users",
			"presentable": false,
			"required": false,
			"system": false,
			"type": "relation"
		}`)); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_1243588648")
		if err != nil {
			return err
		}

		// update collection data
		if err := json.Unmarshal([]byte(`{
			"createRule": null,
			"deleteRule": null,
			"listRule": null,
			"updateRule": null,
			"viewRule": null
		}`), &collection); err != nil {
			return err
		}

		// remove field
		collection.Fields.RemoveById("relation344172009")

		return app.Save(collection)
	})
}
