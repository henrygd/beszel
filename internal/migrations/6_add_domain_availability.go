package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Adds the domains.availability column populated by availability detection
// (RDAP 404 / WHOIS "not found" responses → "available", registration data
// → "registered"). Idempotent for databases created before the field existed.
func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("domains")
		if err != nil {
			return nil
		}
		addTextField(collection, "availability")
		return app.Save(collection)
	}, func(app core.App) error {
		return nil
	})
}
