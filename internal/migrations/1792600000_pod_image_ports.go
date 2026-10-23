package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pods")
		if err != nil {
			return err
		}
		if collection.Fields.GetByName("image") == nil {
			collection.Fields.Add(&core.TextField{Id: "text_pod_image", Name: "image", Max: 2000})
		}
		if collection.Fields.GetByName("ports") == nil {
			collection.Fields.Add(&core.TextField{Id: "text_pod_ports", Name: "ports", Max: 2000})
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pods")
		if err != nil {
			return err
		}
		collection.Fields.RemoveByName("image")
		collection.Fields.RemoveByName("ports")
		return app.Save(collection)
	})
}
