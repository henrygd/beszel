package migrations

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("4afacsdnlu8q8r2")
		if err != nil {
			return err
		}

		// update collection data
		if err := json.Unmarshal([]byte(`{
			"indexes": [
				"CREATE UNIQUE INDEX ` + "`" + `idx_30Lwgf2` + "`" + ` ON ` + "`" + `user_settings` + "`" + ` (` + "`" + `user` + "`" + `)",
				"CREATE UNIQUE INDEX ` + "`" + `idx_PI3JSMeUw5` + "`" + ` ON ` + "`" + `user_settings` + "`" + ` (` + "`" + `api_key` + "`" + `)"
			]
		}`), &collection); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("4afacsdnlu8q8r2")
		if err != nil {
			return err
		}

		// update collection data
		if err := json.Unmarshal([]byte(`{
			"indexes": [
				"CREATE UNIQUE INDEX ` + "`" + `idx_30Lwgf2` + "`" + ` ON ` + "`" + `user_settings` + "`" + ` (` + "`" + `user` + "`" + `)"
			]
		}`), &collection); err != nil {
			return err
		}

		return app.Save(collection)
	})
}
