package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitor_stats")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.NumberField{Id: "network_monitor_stats_loss_max", Name: "loss_max"})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitor_stats")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("loss_max")
		return app.Save(collection)
	})
}
