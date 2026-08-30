package migrations

import (
	"errors"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("network_probes")
		if err != nil {
			return err
		}

		field, ok := collection.Fields.GetByName("protocol").(*core.SelectField)
		if !ok {
			return errors.New("network_probes.protocol field not found or not a select field")
		}

		field.Values = append(field.Values, "dns")

		return app.Save(collection)
	}, func(app core.App) error {
		// No rollback — dev branch only.
		return nil
	})
}
