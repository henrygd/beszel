package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// This migration backfills recovery_modules fields that were added via
// in-place edits to 1712000000_recovery_init.go after that migration had
// already run on real databases - PocketBase migrations run once, so those
// edits never took effect anywhere except a database migrated fresh after
// they landed. Every step is guarded so this is a safe no-op on a database
// that already has the fields (e.g. any fresh install).
func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("recovery_modules")
		if err != nil {
			return err
		}

		// heartbeat_interval was renamed to ping_interval_seconds (to avoid
		// confusion with the unrelated outbound "Heartbeat Monitoring"
		// feature) - rename in place so existing values survive, instead of
		// adding a second field.
		if f, ok := collection.Fields.GetByName("heartbeat_interval").(*core.NumberField); ok {
			f.Name = "ping_interval_seconds"
		}

		type fieldSpec struct {
			id, name, fieldType string
		}
		missing := []fieldSpec{
			{"modulerepr0001", "reported_config_revision", "number"},
			{"modulerephash01", "reported_config_hash", "text"},
			{"modulecfgsrc01", "last_config_source", "text"},
			{"modulependesp1", "pending_esp_change", "bool"},
			{"moduleesppayl1", "esp_change_payload", "json"},
			{"moduletempdis1", "temperature_monitoring_disabled", "bool"},
			{"modulebuzzdis1", "buzzer_disabled", "bool"},
			{"modulebuzzmut1", "buzzer_muted", "bool"},
			{"modulegwonlin1", "gateway_online", "bool"},
		}
		for _, spec := range missing {
			if collection.Fields.GetByName(spec.name) != nil {
				continue
			}
			switch spec.fieldType {
			case "number":
				collection.Fields.Add(&core.NumberField{Id: spec.id, Name: spec.name, OnlyInt: true})
			case "text":
				collection.Fields.Add(&core.TextField{Id: spec.id, Name: spec.name})
			case "bool":
				collection.Fields.Add(&core.BoolField{Id: spec.id, Name: spec.name})
			case "json":
				collection.Fields.Add(&core.JSONField{Id: spec.id, Name: spec.name})
			}
		}

		return app.Save(collection)
	}, func(app core.App) error {
		// No down migration - this only backfills fields that
		// 1712000000_recovery_init.go's own down migration already
		// declines to remove (it's a no-op on rollback).
		return nil
	})
}
