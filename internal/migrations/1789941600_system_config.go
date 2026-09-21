package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		systems, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}

		// systems.agent_config was an earlier dev-only field. Per-system agent
		// settings now live in system_config, with one column per setting.
		if systems.Fields.GetByName("agent_config") != nil {
			systems.Fields.RemoveByName("agent_config")
			if err := app.Save(systems); err != nil {
				return err
			}
		}

		if _, err := app.FindCollectionByNameOrId("system_config"); err == nil {
			return nil
		}

		c := core.NewBaseCollection("system_config")
		c.Fields.Add(
			&core.RelationField{
				Name:          "system",
				CollectionId:  systems.Id,
				Required:      true,
				CascadeDelete: true,
				MaxSelect:     1,
			},
			// Comma-separated container name patterns, same syntax as the EXCLUDE_CONTAINERS env var.
			&core.TextField{Name: "exclude_containers"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		c.AddIndex("idx_system_config_system", true, "system", "")
		return app.Save(c)
	}, func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("system_config")
		if err != nil {
			return nil
		}
		return app.Delete(c)
	})
}
