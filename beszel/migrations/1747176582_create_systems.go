package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		_, err := app.FindCollectionByNameOrId("systems")
		if err == nil { // collection exists, thats update from non migration app version
			return nil
		}
		collection := core.NewBaseCollection("systems")
		collection.ListRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id")
		collection.ViewRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id")
		collection.CreateRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"")
		collection.UpdateRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"")
		collection.DeleteRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"")
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

		collection.Fields.Add(&core.TextField{
			AutogeneratePattern: "[a-z0-9]{15}",
			Hidden:              false,
			Max:                 0,
			Min:                 0,
			Name:                "name",
			Pattern:             "",
			Presentable:         false,
			PrimaryKey:          false,
			Required:            true,
			System:              false,
		})

		collection.Fields.Add(&core.SelectField{
			Hidden:      false,
			MaxSelect:   1,
			Name:        "status",
			Presentable: false,
			Required:    false,
			System:      false,
			Values:      []string{"up", "down", "paused", "pending"},
		})

		collection.Fields.Add(&core.TextField{
			AutogeneratePattern: "",
			Hidden:              false,
			Max:                 0,
			Min:                 0,
			Name:                "host",
			Pattern:             "",
			Presentable:         false,
			PrimaryKey:          false,
			Required:            true,
			System:              false,
		})

		collection.Fields.Add(&core.TextField{
			AutogeneratePattern: "",
			Hidden:              false,
			Max:                 0,
			Min:                 0,
			Name:                "port",
			Pattern:             "",
			Presentable:         false,
			PrimaryKey:          false,
			Required:            true,
			System:              false,
		})

		collection.Fields.Add(&core.JSONField{
			Hidden:      false,
			MaxSize:     2000000,
			Name:        "info",
			Presentable: false,
			Required:    false,
			System:      false,
		})

		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.RelationField{
			CascadeDelete: true,
			CollectionId:  usersCollection.Id,
			Hidden:        false,
			MaxSelect:     2147483647,
			MinSelect:     0,
			Name:          "users",
			Presentable:   true,
			Required:      true,
			System:        false,
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

		return app.Save(collection)
	}, nil)
}
