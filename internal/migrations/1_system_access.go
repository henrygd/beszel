package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Remove users relation field from systems collection.
		// Rules referencing users.id must be cleared first so PocketBase validation
		// doesn't reject the save — setCollectionAuthSettings will re-apply them on start.
		systemsCollection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		systemsCollection.Fields.RemoveById("jcarjnjj") // users relation field id
		systemsCollection.ListRule = nil
		systemsCollection.ViewRule = nil
		systemsCollection.CreateRule = nil
		systemsCollection.UpdateRule = nil
		systemsCollection.DeleteRule = nil
		return app.Save(systemsCollection)
	}, nil)
}
