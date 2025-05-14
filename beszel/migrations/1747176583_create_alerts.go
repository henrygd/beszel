package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		collection := core.NewBaseCollection("alerts")
		collection.ListRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")
		collection.ViewRule = types.Pointer("")
		collection.CreateRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")
		collection.UpdateRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")
		collection.DeleteRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")
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

		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.RelationField{
			CascadeDelete: true,
			CollectionId:  usersCollection.Id,
			Hidden:        false,
			MaxSelect:     1,
			MinSelect:     0,
			Name:          "user",
			Presentable:   false,
			Required:      true,
			System:        false,
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

		collection.Fields.Add(&core.SelectField{
			Hidden:      false,
			MaxSelect:   1,
			Name:        "name",
			Presentable: false,
			Required:    true,
			System:      false,
			Values: []string{
				"Status",
				"CPU",
				"Memory",
				"Disk",
				"Temperature",
				"Bandwidth",
			},
		})

		collection.Fields.Add(&core.NumberField{
			Hidden:      false,
			Max:         nil,
			Min:         nil,
			Name:        "value",
			OnlyInt:     false,
			Presentable: false,
			Required:    false,
			System:      false,
		})

		collection.Fields.Add(&core.NumberField{
			Hidden:      false,
			Max:         types.Pointer(60.0),
			Min:         nil,
			Name:        "min",
			OnlyInt:     true,
			Presentable: false,
			Required:    false,
			System:      false,
		})

		collection.Fields.Add(&core.BoolField{
			Hidden:      false,
			Name:        "triggered",
			Presentable: false,
			Required:    false,
			System:      false,
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
			"CREATE UNIQUE INDEX `idx_MnhEt21L5r` ON `alerts` (`user`, `system`, `name`)",
		}

		return app.Save(collection)
	}, nil)
}
