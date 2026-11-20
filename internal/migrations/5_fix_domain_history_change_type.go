package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// The domain_history.change_type select was defined before the scheduler
// learned "provider", "security" and "seo" change types. Saving history
// entries with those values fails validation on databases created from the
// old definition. Expand the select to the full set the code emits.
func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("domain_history")
		if err != nil {
			return nil // collection absent; nothing to fix
		}
		field := collection.Fields.GetByName("change_type")
		sel, ok := field.(*core.SelectField)
		if !ok || sel == nil {
			return nil
		}
		want := map[string]bool{
			"expiry": true, "ssl": true, "dns": true, "registrar": true,
			"ip": true, "host": true, "status": true, "provider": true,
			"security": true, "seo": true,
		}
		changed := false
		for _, v := range []string{"provider", "security", "seo"} {
			if !want[v] {
				continue
			}
			found := false
			for _, existing := range sel.Values {
				if existing == v {
					found = true
					break
				}
			}
			if !found {
				sel.Values = append(sel.Values, v)
				changed = true
			}
		}
		if !changed {
			return nil
		}
		return app.Save(collection)
	}, func(app core.App) error {
		return nil // down: leave expanded values
	})
}
