package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
		{
			"name": "alerts_history",
			"type": "base",
			"system": false,
			"listRule": "",
			"deleteRule": "",
			"viewRule": ""
			"fields": [
				{
					"name": "alert",
					"type": "relation",
					"required": true,
					"collectionId": "elngm8x1l60zi2v",
					"cascadeDelete": true,
					"maxSelect": 1
				},
				{
					"name": "user",
					"type": "relation",
					"required": true,
					"collectionId": "_pb_users_auth_",
					"cascadeDelete": true,
					"maxSelect": 1
				},
				{
					"name": "system",
					"type": "relation",
					"required": true,
					"collectionId": "2hz5ncl8tizk5nx",
					"cascadeDelete": true,
					"maxSelect": 1
				},
				{
					"name": "name",
					"type": "text",
					"required": true
				},
				{
					"name": "value",
					"type": "number",
					"required": true
				},
				{
					"name": "state",
					"type": "select",
					"required": true,
					"values": ["active", "solved"]
				},
				{
					"name": "created_date",
					"type": "date",
					"required": true
				},
				{
					"name": "solved_date",
					"type": "date",
					"required": false
				}
			]
		}
	]`
		return app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
	}, nil)
}
