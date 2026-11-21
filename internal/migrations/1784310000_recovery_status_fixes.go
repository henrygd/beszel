package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// 1. last_ping: dedicated heartbeat timestamp on recovery_modules,
		// written only by handleRecoveryPing. The autodate `updated` field is
		// bumped by ANY record save (including UI edits), so it cannot be
		// trusted as a liveness signal.
		modules, err := app.FindCollectionByNameOrId("recovery_modules")
		if err != nil {
			return err
		}
		modules.Fields.Add(&core.DateField{
			Id:   "modlastping001",
			Name: "last_ping",
		})
		if err := app.Save(modules); err != nil {
			return err
		}

		// 2. One system may be protected by at most one relay channel.
		// Dedupe existing rows first (keep the earliest mapping per system)
		// so the unique index can be created on already-populated databases.
		if _, err := app.DB().NewQuery(
			"DELETE FROM recovery_channels WHERE rowid NOT IN (SELECT MIN(rowid) FROM recovery_channels GROUP BY system)",
		).Execute(); err != nil {
			return err
		}

		channels, err := app.FindCollectionByNameOrId("recovery_channels")
		if err != nil {
			return err
		}
		channels.AddIndex("idx_recovery_channels_system_unique", true, "`system`", "")

		// 3. probe_ports is no longer used for probing (reachability is now
		// checked with ICMP ping against the host IP), so stop requiring it.
		if f, ok := channels.Fields.GetByName("probe_ports").(*core.JSONField); ok {
			f.Required = false
		}

		return app.Save(channels)
	}, func(app core.App) error {
		modules, err := app.FindCollectionByNameOrId("recovery_modules")
		if err != nil {
			return err
		}
		modules.Fields.RemoveById("modlastping001")
		if err := app.Save(modules); err != nil {
			return err
		}

		channels, err := app.FindCollectionByNameOrId("recovery_channels")
		if err != nil {
			return err
		}
		channels.RemoveIndex("idx_recovery_channels_system_unique")
		if f, ok := channels.Fields.GetByName("probe_ports").(*core.JSONField); ok {
			f.Required = true
		}
		return app.Save(channels)
	})
}
