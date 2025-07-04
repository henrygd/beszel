package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
		{
			"name": "systems",
			"fields": [
				{
					"id": "tags1",
					"name": "tags",
					"type": "json",
					"required": false,
					"system": false,
					"hidden": false,
					"presentable": false,
					"options": {}
				}
			]
		}
		]`
		return app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
	}, nil)
}
