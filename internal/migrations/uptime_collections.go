package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func fnum(v float64) *float64 {
	return &v
}

func init() {
	m.Register(func(app core.App) error {
		if _, err := app.FindCollectionByNameOrId("monitors"); err == nil {
			return nil // already created
		}

		// user collection (for the relation field)
		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		// ------------------------------------------------------
		// monitors collection
		// ------------------------------------------------------
		monitors := core.NewBaseCollection("monitors")

		monitors.Fields = append(monitors.Fields,
			&core.RelationField{
				Name:          "user",
				CollectionId:  usersCollection.Id,
				CascadeDelete: false,
			},
			&core.TextField{
				Name:     "name",
				Required: true,
				Max:      150,
			},
			&core.SelectField{
				Name:        "type",
				Required:    true,
				Values:      []string{"http", "tcp", "ping"},
				Presentable: true,
			},
			&core.TextField{
				Name: "url",
				Max:  2048,
				Min:  3,
			},
			&core.TextField{
				Name: "host",
				Max:  255,
			},
			&core.NumberField{
				Name:    "port",
				Min:     fnum(1),
				Max:     fnum(65535),
				OnlyInt: true,
			},
			&core.NumberField{
				Name:    "interval",
				Min:     fnum(5),
				Max:     fnum(86400),
				OnlyInt: true,
			},
			&core.NumberField{
				Name:    "timeout",
				Min:     fnum(1),
				Max:     fnum(120),
				OnlyInt: true,
			},
			&core.BoolField{
				Name: "retry",
			},
			&core.BoolField{
				Name: "secure",
			},
			&core.SelectField{
				Name:   "method",
				Values: []string{"get", "post", "put", "delete", "head", "patch"},
			},
			&core.TextField{
				Name: "expected_status",
				Max:  3,
			},
			&core.TextField{
				Name: "expected_body",
				Max:  512,
			},
			&core.JSONField{
				Name: "headers",
			},
			&core.SelectField{
				Name:   "status",
				Values: []string{"up", "down", "paused", "pending"},
			},
			&core.NumberField{
				Name:    "num_retries",
				Min:     fnum(1),
				Max:     fnum(20),
				OnlyInt: true,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)

		monitors.AddIndex("idx_monitors_user", false, "user", "")

		if err := app.Save(monitors); err != nil {
			return err
		}

		// ------------------------------------------------------
		// monitor_checks collection (check history)
		// ------------------------------------------------------
		checks := core.NewBaseCollection("monitor_checks")
		checks.Fields = append(checks.Fields,
			&core.RelationField{
				Name:          "monitor",
				CollectionId:  monitors.Id,
				CascadeDelete: true,
			},
			&core.BoolField{
				Name: "up",
			},
			&core.NumberField{
				Name: "ms",
				Min:  fnum(0),
			},
			&core.TextField{
				Name: "msg",
				Max:  512,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
		)
		checks.AddIndex("idx_checks_monitor_created", false, "monitor, created", "")

		return app.Save(checks)
	}, nil)
}
