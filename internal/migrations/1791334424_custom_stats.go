package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// custom_stats holds the values of custom metrics, shaped like container_stats:
// one row per system and record type, with stats keyed by metric.
func init() {
	m.Register(func(app core.App) error {
		systems, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		collection := core.NewBaseCollection("custom_stats")
		collection.Fields.Add(
			&core.RelationField{Id: "cs_system", Name: "system", CollectionId: systems.Id, CascadeDelete: true, MaxSelect: 1, Required: true},
			&core.JSONField{Id: "cs_stats", Name: "stats", MaxSize: 2000000, Required: true},
			&core.SelectField{Id: "cs_type", Name: "type", MaxSelect: 1, Required: true, Values: []string{"1m", "10m", "20m", "120m", "480m"}},
			&core.AutodateField{Id: "cs_created", Name: "created", OnCreate: true},
			&core.AutodateField{Id: "cs_updated", Name: "updated", OnCreate: true, OnUpdate: true},
		)
		collection.AddIndex("idx_custom_stats_system_type_created", false, "`system`, `type`, `created`", "")
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("custom_stats")
		if err != nil {
			return err
		}
		return app.Delete(collection)
	})
}
