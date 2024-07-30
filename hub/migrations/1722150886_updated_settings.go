package migrations

import (
	"encoding/json"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/models/schema"
)

func init() {
	m.Register(func(db dbx.Builder) error {
		dao := daos.New(db);

		collection, err := dao.FindCollectionByNameOrId("7z37bxsy3rslxwd")
		if err != nil {
			return err
		}

		// add
		new_enum := &schema.SchemaField{}
		if err := json.Unmarshal([]byte(`{
			"system": false,
			"id": "yxmbzxjl",
			"name": "enum",
			"type": "number",
			"required": false,
			"presentable": false,
			"unique": false,
			"options": {
				"min": null,
				"max": null,
				"noDecimal": false
			}
		}`), new_enum); err != nil {
			return err
		}
		collection.Schema.AddField(new_enum)

		return dao.SaveCollection(collection)
	}, func(db dbx.Builder) error {
		dao := daos.New(db);

		collection, err := dao.FindCollectionByNameOrId("7z37bxsy3rslxwd")
		if err != nil {
			return err
		}

		// remove
		collection.Schema.RemoveField("yxmbzxjl")

		return dao.SaveCollection(collection)
	})
}
