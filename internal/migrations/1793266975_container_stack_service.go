package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Containers are grouped by stack, and a stack spans several systems, so the index leads with
// system to keep the per-system queries the table already serves.
const containerStackIndex = "CREATE INDEX `idx_containers_stack` ON `containers` (`system`, `stack`)"

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("containers")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("stack") == nil {
			collection.Fields.Add(&core.TextField{Id: "containers_stack", Name: "stack"})
		}
		if collection.Fields.GetByName("service") == nil {
			collection.Fields.Add(&core.TextField{Id: "containers_service", Name: "service"})
		}
		if !slices.Contains(collection.Indexes, containerStackIndex) {
			collection.Indexes = append(collection.Indexes, containerStackIndex)
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("containers")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("stack")
		collection.Fields.RemoveByName("service")
		collection.Indexes = slices.DeleteFunc(collection.Indexes, func(index string) bool {
			return index == containerStackIndex
		})
		return app.Save(collection)
	})
}
