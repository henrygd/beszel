package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		collection := core.NewBaseCollection("user_settings")
		collection.ListRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")
		collection.ViewRule = nil
		collection.CreateRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")
		collection.UpdateRule = types.Pointer("@request.auth.id != \"\" && user.id = @request.auth.id")
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

		collection.Fields.Add(&core.JSONField{
			Hidden:      false,
			MaxSize:     2000000,
			Name:        "settings",
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
			"CREATE UNIQUE INDEX `idx_30Lwgf2` ON `user_settings` (`user`)",
		}

		return app.Save(collection)
	}, nil)
}
