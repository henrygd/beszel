package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// 1_monitors.go creates the monitors and monitor_checks collections for
// external uptime monitoring (PR1: hub-only). The agent field is reserved
// nullable for PR2 (distributed checks) and must stay unset in PR1.
func init() {
	m.Register(func(app core.App) error {
		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		systemsCollection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}

		monitors := core.NewBaseCollection("monitors")
		monitors.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 100},
			&core.SelectField{Name: "type", Required: true, MaxSelect: 1, Values: []string{"http", "keyword", "ping", "dns", "tls"}},
			&core.TextField{Name: "target", Required: true, Max: 500},
			&core.NumberField{Name: "interval", Required: true, OnlyInt: true, Min: types.Pointer(20.0), Max: types.Pointer(86400.0)},
			&core.NumberField{Name: "timeout", Required: true, OnlyInt: true, Min: types.Pointer(1.0)},
			&core.NumberField{Name: "max_retries", OnlyInt: true, Min: types.Pointer(0.0), Max: types.Pointer(10.0)},
			&core.BoolField{Name: "upside_down"},
			&core.BoolField{Name: "paused"},
			&core.BoolField{Name: "notify"},
			&core.NumberField{Name: "resend_after", OnlyInt: true, Min: types.Pointer(0.0), Max: types.Pointer(1440.0)},
			&core.JSONField{Name: "config", MaxSize: 2000000},
			&core.SelectField{Name: "status", MaxSelect: 1, Values: []string{"up", "down", "warn", "paused", "pending"}},
			&core.DateField{Name: "last_check"},
			&core.NumberField{Name: "last_latency_ms"},
			&core.NumberField{Name: "uptime_24h"},
			&core.NumberField{Name: "cert_days"},
			&core.NumberField{Name: "consecutive_failures", OnlyInt: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		monitors.Fields.Add(&core.RelationField{
			Name: "users", Required: true, MaxSelect: 0,
			CollectionId: usersCollection.Id,
		})
		monitors.Fields.Add(&core.RelationField{
			Name: "agent", MaxSelect: 1, MinSelect: 0,
			CollectionId: systemsCollection.Id,
		})
		// Read: members or SHARE_ALL (refined at serve time like systems).
		// Write: set by API layer; base rules require auth.
		monitors.ListRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id")
		monitors.ViewRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id")
		monitors.CreateRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"")
		monitors.UpdateRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"")
		monitors.DeleteRule = types.Pointer("@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"")
		if err := app.Save(monitors); err != nil {
			return err
		}

		checks := core.NewBaseCollection("monitor_checks")
		checks.Fields.Add(
			&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"up", "down", "warn"}},
			&core.NumberField{Name: "latency_ms"},
			&core.NumberField{Name: "code"},
			&core.TextField{Name: "message", Max: 500},
			&core.JSONField{Name: "details", MaxSize: 2000000},
			&core.NumberField{Name: "cert_days"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		checks.Fields.Add(&core.RelationField{
			Name: "monitor", Required: true, MaxSelect: 1,
			CollectionId: monitors.Id, CascadeDelete: true,
		})
		// History is server-written; no public create/update/delete.
		checks.ListRule = types.Pointer("@request.auth.id != \"\" && monitor.users.id ?= @request.auth.id")
		checks.ViewRule = types.Pointer("@request.auth.id != \"\" && monitor.users.id ?= @request.auth.id")
		checks.AddIndex("idx_monitor_checks_monitor_created", false, "monitor", "created")
		// AddIndex(name, unique, columns, where): "" where = no condition.
		checks.AddIndex("idx_monitor_checks_created", false, "created", "")
		return app.Save(checks)
	}, func(app core.App) error {
		if checks, err := app.FindCollectionByNameOrId("monitor_checks"); err == nil {
			if err := app.Delete(checks); err != nil {
				return err
			}
		}
		if monitors, err := app.FindCollectionByNameOrId("monitors"); err == nil {
			return app.Delete(monitors)
		}
		return nil
	})
}
