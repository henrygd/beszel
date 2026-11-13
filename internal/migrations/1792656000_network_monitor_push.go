package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const networkMonitorPushProtocol = "push"

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		protocol := collection.Fields.GetByName("protocol").(*core.SelectField)
		if !slices.Contains(protocol.Values, networkMonitorPushProtocol) {
			protocol.Values = append(protocol.Values, networkMonitorPushProtocol)
		}
		// Push monitors may expect heartbeats as rarely as once a day. The hub
		// still limits probe protocols to one hour.
		collection.Fields.GetByName("interval").(*core.NumberField).Max = new(float64(86400))
		collection.Fields.Add(
			&core.TextField{Id: "nm_push_token", Name: "pushToken", Max: 64},
			&core.NumberField{Id: "nm_last_push", Name: "lastPush", OnlyInt: true},
		)
		collection.AddIndex("idx_nm_push_token", true, "pushToken", "pushToken != ''")
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.RemoveIndex("idx_nm_push_token")
		collection.Fields.RemoveByName("pushToken")
		collection.Fields.RemoveByName("lastPush")
		protocol := collection.Fields.GetByName("protocol").(*core.SelectField)
		protocol.Values = slices.DeleteFunc(protocol.Values, func(v string) bool { return v == networkMonitorPushProtocol })
		collection.Fields.GetByName("interval").(*core.NumberField).Max = new(float64(3600))
		return app.Save(collection)
	})
}
