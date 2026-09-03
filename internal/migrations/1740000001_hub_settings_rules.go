package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("hub_settings")
		if err != nil {
			return nil
		}
		// tighten read access to admin-only; unauthenticated and non-admin can use /api/beszel/retention
		adminRule := "@request.auth.role = 'admin'"
		col.ListRule = &adminRule
		col.ViewRule = &adminRule
		return app.Save(col)
	}, func(app core.App) error {
		col, err := app.FindCollectionByNameOrId("hub_settings")
		if err != nil {
			return nil
		}
		rule := "@request.auth.id != \"\""
		col.ListRule = &rule
		col.ViewRule = &rule
		return app.Save(col)
	})
}
