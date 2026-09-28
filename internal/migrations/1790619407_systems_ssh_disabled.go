package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		// place before the autodate fields (appends if "created" is missing)
		pos := slices.Index(collection.Fields.FieldNames(), "created")
		collection.Fields.AddAt(pos, &core.BoolField{Id: "sys_ssh_disabled", Name: "ssh_disabled"})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("ssh_disabled")
		return app.Save(collection)
	})
}
