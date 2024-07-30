package migrations

import (
	"encoding/json"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/models/schema"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(db dbx.Builder) error {
		dao := daos.New(db);

		collection, err := dao.FindCollectionByNameOrId("7z37bxsy3rslxwd")
		if err != nil {
			return err
		}

		collection.ListRule = types.Pointer("@request.auth.id != \"\"")

		collection.ViewRule = types.Pointer("@request.auth.id != \"\"")

		collection.CreateRule = types.Pointer("@request.auth.id != \"\"")

		collection.UpdateRule = types.Pointer("@request.auth.id != \"\"")

		collection.DeleteRule = types.Pointer("@request.auth.id != \"\"")

		// remove
		collection.Schema.RemoveField("suqfpxbu")

		return dao.SaveCollection(collection)
	}, func(db dbx.Builder) error {
		dao := daos.New(db);

		collection, err := dao.FindCollectionByNameOrId("7z37bxsy3rslxwd")
		if err != nil {
			return err
		}

		collection.ListRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")

		collection.ViewRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")

		collection.CreateRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")

		collection.UpdateRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")

		collection.DeleteRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")

		// add
		del_user := &schema.SchemaField{}
		if err := json.Unmarshal([]byte(`{
			"system": false,
			"id": "suqfpxbu",
			"name": "user",
			"type": "relation",
			"required": false,
			"presentable": false,
			"unique": false,
			"options": {
				"collectionId": "_pb_users_auth_",
				"cascadeDelete": false,
				"minSelect": null,
				"maxSelect": 1,
				"displayFields": null
			}
		}`), del_user); err != nil {
			return err
		}
		collection.Schema.AddField(del_user)

		return dao.SaveCollection(collection)
	})
}
