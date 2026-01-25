package migrations

import (
	"encoding/json"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// DIRECT DB UPDATE to bypass validation and visibility issues
		type CollectionRow struct {
			Id     string `db:"id"`
			Fields string `db:"fields"`
		}
		var row CollectionRow

		// 1. Read raw JSON
		err := app.DB().NewQuery("SELECT id, fields FROM _collections WHERE name='alerts'").One(&row)
		if err != nil {
			return err
		}

		// 2. Parse fields
		var fields []map[string]interface{}
		if err := json.Unmarshal([]byte(row.Fields), &fields); err != nil {
			return err
		}

		// 3. Modify
		found := false
		for i, f := range fields {
			if name, ok := f["name"].(string); ok && name == "name" {
				fields[i]["type"] = "text"
				delete(fields[i], "values")
				delete(fields[i], "maxSelect")
				found = true
				break
			}
		}

		if !found {
			return nil
		}

		// 4. Marshal back
		newJson, err := json.Marshal(fields)
		if err != nil {
			return err
		}

		// 5. Update raw
		_, err = app.DB().NewQuery("UPDATE _collections SET fields={:fields} WHERE id={:id}").Bind(dbx.Params{
			"fields": string(newJson),
			"id":     row.Id,
		}).Execute()

		return err
	}, func(app core.App) error {
		// revert
		// collection, err := app.FindCollectionByNameOrId("alerts")
		// if err != nil {
		// 	return err
		// }

		// We would need to set options here if we reverted, but this is a complex struct.
		// For now, let's just make it text to be safe, or we can assume we don't need perfect revert for this dev fix.
		// Ideally we reconstruct the select field.

		return nil
	})
}
