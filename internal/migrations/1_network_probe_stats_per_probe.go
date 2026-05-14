package migrations

import (
	"encoding/json"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/security"
)

func init() {
	m.Register(func(app core.App) error {
		// Add probe relation field and new index to network_probe_stats.
		// probe is optional here to allow the backfill below to complete before records go required.
		err := app.ImportCollectionsByMarshaledJSON([]byte(`[{
			"id": "np_stats_001",
			"name": "network_probe_stats",
			"type": "base",
			"fields": [
				{
					"autogeneratePattern": "[a-z0-9]{10}",
					"hidden": false,
					"id": "text3208210256",
					"max": 10,
					"min": 10,
					"name": "id",
					"pattern": "^[a-z0-9]+$",
					"presentable": false,
					"primaryKey": true,
					"required": true,
					"system": true,
					"type": "text"
				},
				{
					"cascadeDelete": true,
					"collectionId": "2hz5ncl8tizk5nx",
					"hidden": false,
					"id": "nps_system",
					"maxSelect": 1,
					"minSelect": 0,
					"name": "system",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "relation"
				},
				{
					"cascadeDelete": true,
					"collectionId": "np_probes_001",
					"hidden": false,
					"id": "nps_probe",
					"maxSelect": 1,
					"minSelect": 0,
					"name": "probe",
					"presentable": false,
					"required": false,
					"system": false,
					"type": "relation"
				},
				{
					"hidden": false,
					"id": "nps_stats",
					"maxSize": 2000000,
					"name": "stats",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "json"
				},
				{
					"hidden": false,
					"id": "nps_type",
					"maxSelect": 1,
					"name": "type",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "select",
					"values": ["1m", "10m", "20m", "120m", "480m"]
				},
				{
					"hidden": false,
					"id": "number2990389176",
					"max": null,
					"min": null,
					"name": "created",
					"onlyInt": false,
					"presentable": false,
					"required": false,
					"system": false,
					"type": "number"
				}
			],
			"indexes": [
				"CREATE INDEX IF NOT EXISTS ` + "`" + `idx_nps_system_type_created` + "`" + ` ON ` + "`" + `network_probe_stats` + "`" + ` (` + "`" + `system` + "`" + `, ` + "`" + `type` + "`" + `, ` + "`" + `created` + "`" + `)",
				"CREATE INDEX IF NOT EXISTS ` + "`" + `idx_nps_probe_type_created` + "`" + ` ON ` + "`" + `network_probe_stats` + "`" + ` (` + "`" + `probe` + "`" + `, ` + "`" + `type` + "`" + `, ` + "`" + `created` + "`" + `)"
			],
			"listRule": null,
			"viewRule": null,
			"createRule": null,
			"updateRule": null,
			"deleteRule": null
		}]`), false)
		if err != nil {
			return err
		}

		// Backfill: explode old per-system map records into individual per-probe records.
		// Old format: { system, stats: {"probeId": [avg,min,max,loss], ...}, type, created }
		// New format: { system, probe, stats: [avg,min,max,loss], type, created }
		db := app.DB()

		type oldRecord struct {
			Id      string `db:"id"`
			System  string `db:"system"`
			Stats   []byte `db:"stats"`
			Type    string `db:"type"`
			Created int64  `db:"created"`
		}
		var oldRecords []oldRecord
		if err := db.NewQuery("SELECT id, system, stats, type, created FROM network_probe_stats WHERE probe IS NULL OR probe = ''").All(&oldRecords); err != nil {
			return err
		}
		if len(oldRecords) == 0 {
			return nil
		}

		// Collect valid probe IDs to skip orphaned entries.
		type probeRow struct {
			Id string `db:"id"`
		}
		var probeRows []probeRow
		if err := db.NewQuery("SELECT id FROM network_probes").All(&probeRows); err != nil {
			return err
		}
		validProbes := make(map[string]bool, len(probeRows))
		for _, p := range probeRows {
			validProbes[p.Id] = true
		}

		for _, rec := range oldRecords {
			var statsMap map[string][]float64
			if err := json.Unmarshal(rec.Stats, &statsMap); err != nil {
				// Not a map — already in new format or corrupt; skip.
				continue
			}
			for probeId, stats := range statsMap {
				if !validProbes[probeId] {
					continue
				}
				statsJSON, err := json.Marshal(stats)
				if err != nil {
					continue
				}
				newId := security.PseudorandomStringWithAlphabet(10, core.DefaultIdAlphabet)
				_, _ = db.Insert("network_probe_stats", dbx.Params{
					"id":      newId,
					"system":  rec.System,
					"probe":   probeId,
					"stats":   string(statsJSON),
					"type":    rec.Type,
					"created": rec.Created,
				}).Execute()
			}
			// Remove the old aggregated record.
			_, _ = db.NewQuery("DELETE FROM network_probe_stats WHERE id = {:id}").
				Bind(dbx.Params{"id": rec.Id}).Execute()
		}

		return nil
	}, func(app core.App) error {
		// No rollback — dev branch only.
		return nil
	})
}
