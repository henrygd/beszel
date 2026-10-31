package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// nut_stats keeps UPS/PDU measurements in the same time tiers as system metrics.
func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
			{
				"id": "pbc_nut_stats0001",
				"name": "nut_stats",
				"type": "base",
				"system": false,
				"listRule": null,
				"viewRule": null,
				"createRule": null,
				"updateRule": null,
				"deleteRule": null,
				"fields": [
					{"id":"text3208210256","name":"id","type":"text","system":true,"required":true,"primaryKey":true,"hidden":false,"presentable":false,"min":15,"max":15,"pattern":"^[a-z0-9]+$","autogeneratePattern":"[a-z0-9]{15}"},
					{"id":"nutstatssystem01","name":"system","type":"relation","system":false,"required":true,"hidden":false,"presentable":false,"collectionId":"2hz5ncl8tizk5nx","cascadeDelete":true,"minSelect":0,"maxSelect":1},
					{"id":"nutstatsdevice01","name":"device","type":"text","system":false,"required":true,"hidden":false,"presentable":false,"min":0,"max":15,"pattern":"","autogeneratePattern":""},
					{"id":"nutstatstype0001","name":"type","type":"select","system":false,"required":true,"hidden":false,"presentable":false,"maxSelect":1,"values":["1m","10m","20m","120m","480m"]},
					{"id":"nutstatscharge01","name":"battery_charge","type":"number","system":false,"required":false,"hidden":false,"presentable":false,"min":null,"max":null,"onlyInt":false},
					{"id":"nutstatsruntime1","name":"battery_runtime","type":"number","system":false,"required":false,"hidden":false,"presentable":false,"min":null,"max":null,"onlyInt":false},
					{"id":"nutstatsbatvolt01","name":"battery_voltage","type":"number","system":false,"required":false,"hidden":false,"presentable":false,"min":null,"max":null,"onlyInt":false},
					{"id":"nutstatsinputv001","name":"input_voltage","type":"number","system":false,"required":false,"hidden":false,"presentable":false,"min":null,"max":null,"onlyInt":false},
					{"id":"nutstatsoutputv01","name":"output_voltage","type":"number","system":false,"required":false,"hidden":false,"presentable":false,"min":null,"max":null,"onlyInt":false},
					{"id":"nutstatsload0001","name":"load","type":"number","system":false,"required":false,"hidden":false,"presentable":false,"min":null,"max":null,"onlyInt":false},
					{"id":"nutstatscurrent01","name":"output_current","type":"number","system":false,"required":false,"hidden":false,"presentable":false,"min":null,"max":null,"onlyInt":false},
					{"id":"nutstatspower0001","name":"output_power","type":"number","system":false,"required":false,"hidden":false,"presentable":false,"min":null,"max":null,"onlyInt":false},
					{"id":"autodate2990389176","name":"created","type":"autodate","system":false,"required":false,"hidden":false,"presentable":false,"onCreate":true,"onUpdate":false},
					{"id":"autodate3332085495","name":"updated","type":"autodate","system":false,"required":false,"hidden":false,"presentable":false,"onCreate":true,"onUpdate":true}
				],
				"indexes": ["CREATE INDEX idx_nut_stats_system_device_type_created ON nut_stats (system, device, type, created)"]
			}
		]`
		return app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("nut_stats")
		if err != nil {
			return err
		}
		return app.Delete(collection)
	})
}
