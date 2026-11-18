package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("zfs_pools")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.TextField{Id: "zfs_pool_status", Name: "status"})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("zfs_pools")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("status")
		return app.Save(collection)
	})
}
