package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.TextField{
			Id:   "nm_status_codes",
			Name: "statusCodes",
			Max:  200,
			// Comma-separated status codes and ranges, such as "200-299,401".
			Pattern: `^[1-5]\d{2}(-[1-5]\d{2})?(,[1-5]\d{2}(-[1-5]\d{2})?)*$`,
		})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("statusCodes")
		return app.Save(collection)
	})
}
