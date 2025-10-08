package migrations

import (
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// This can be deleted after Nov 2025 or so

func init() {
	m.Register(func(app core.App) error {
		app.RunInTransaction(func(txApp core.App) error {
			var systemIds []string
			txApp.DB().NewQuery("SELECT id FROM systems").Column(&systemIds)

			for _, systemId := range systemIds {
				var statRecordIds []string
				txApp.DB().NewQuery("SELECT id FROM system_stats WHERE system = {:system} AND created > {:created}").Bind(map[string]any{"system": systemId, "created": "2025-09-21"}).Column(&statRecordIds)

				for _, statRecordId := range statRecordIds {
					statRecord, err := txApp.FindRecordById("system_stats", statRecordId)
					if err != nil {
						return err
					}
					var systemStats system.Stats
					err = statRecord.UnmarshalJSONField("stats", &systemStats)
					if err != nil {
						return err
					}
					// if mem buff cache is less than total mem, we don't need to fix it
					if systemStats.MemBuffCache < systemStats.Mem {
						continue
					}
					systemStats.MemBuffCache = 0
					statRecord.Set("stats", systemStats)
					err = txApp.SaveNoValidate(statRecord)
					if err != nil {
						return err
					}
				}
			}

			return nil
		})
		return nil
	}, func(app core.App) error {
		return nil
	})
}
