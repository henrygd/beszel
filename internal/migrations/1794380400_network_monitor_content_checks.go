package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.Fields.Add(&core.TextField{Id: "nm_keyword", Name: "keyword", Max: 500})
		collection.Fields.Add(&core.TextField{Id: "nm_json_path", Name: "jsonPath", Max: 200})
		collection.Fields.Add(&core.SelectField{
			Id:        "nm_record_type",
			Name:      "recordType",
			MaxSelect: 1,
			Values:    []string{"A", "AAAA", "CNAME", "MX", "NS", "TXT"},
		})
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_monitors")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("keyword")
		collection.Fields.RemoveByName("jsonPath")
		collection.Fields.RemoveByName("recordType")
		return app.Save(collection)
	})
}
