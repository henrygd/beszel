package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("containers")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.TextField{
			Id:   "text_compose_project",
			Name: "compose",
		})
		collection.Fields.Add(&core.NumberField{
			Id:      "number_traffic_out",
			Name:    "traffic_out",
			OnlyInt: true,
		})
		collection.Fields.Add(&core.NumberField{
			Id:      "number_traffic_in",
			Name:    "traffic_in",
			OnlyInt: true,
		})
		return app.Save(collection)
	}, nil)
}
