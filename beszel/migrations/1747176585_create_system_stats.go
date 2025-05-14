package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		_, err := app.FindCollectionByNameOrId("system_stats")
		if err == nil { // collection exists, thats update from non migration app version
			return nil
		}
		collection := core.NewBaseCollection("system_stats")
		collection.ListRule = types.Pointer("@request.auth.id != \"\"")
		collection.ViewRule = nil
		collection.CreateRule = nil
		collection.UpdateRule = nil
		collection.DeleteRule = nil
		collection.Type = core.CollectionTypeBase
		collection.System = false

		collection.Fields.Add(&core.TextField{
			AutogeneratePattern: "[a-z0-9]{15}",
			Hidden:              false,
			Max:                 15,
			Min:                 15,
			Name:                "id",
			Pattern:             "^[a-z0-9]+$",
			Presentable:         false,
			PrimaryKey:          true,
			Required:            true,
			System:              true,
		})

		systemsCollection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.RelationField{
			CascadeDelete: true,
			CollectionId:  systemsCollection.Id,
			Hidden:        false,
			MaxSelect:     1,
			MinSelect:     0,
			Name:          "system",
			Presentable:   false,
			Required:      true,
			System:        false,
		})

		collection.Fields.Add(&core.JSONField{
			Hidden:      false,
			MaxSize:     2000000,
			Name:        "stats",
			Presentable: false,
			Required:    true,
			System:      false,
		})

		collection.Fields.Add(&core.SelectField{
			Hidden:      false,
			MaxSelect:   1,
			Name:        "type",
			Presentable: false,
			Required:    true,
			System:      false,
			Values: []string{
				"1m",
				"10m",
				"20m",
				"120m",
				"480m",
			},
		})

		collection.Fields.Add(&core.AutodateField{
			Hidden:      false,
			Name:        "created",
			OnCreate:    true,
			OnUpdate:    false,
			Presentable: false,
			System:      false,
		})

		collection.Fields.Add(&core.AutodateField{
			Hidden:      false,
			Name:        "updated",
			OnCreate:    true,
			OnUpdate:    true,
			Presentable: false,
			System:      false,
		})

		collection.Indexes = []string{
			"CREATE INDEX `idx_GxIee0j` ON `system_stats` (`system`, `type`, `created`)",
		}

		return app.Save(collection)
	}, nil)
}
