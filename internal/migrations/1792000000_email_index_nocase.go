package migrations

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/dbutils"
)

// Makes email login case-insensitive (#1887) by adding COLLATE NOCASE to the
// unique email index of the auth collections. PocketBase's
// FindAuthRecordByEmail then performs a case-insensitive lookup automatically.
func init() {
	m.Register(func(app core.App) error {
		return setEmailIndexCollation(app, "NOCASE")
	}, func(app core.App) error {
		return setEmailIndexCollation(app, "")
	})
}

var emailNocaseCollections = []string{"users", core.CollectionNameSuperusers}

func setEmailIndexCollation(app core.App, collate string) error {
	for _, name := range emailNocaseCollections {
		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return err
		}

		// A NOCASE unique index can't be created if emails that differ only
		// by case already exist. Don't block the upgrade (or lock anyone out);
		// keep the existing case-sensitive index and log a warning so the
		// admin can resolve the duplicates. A later release can retry.
		if collate != "" {
			var dupes []struct {
				Email string `db:"email"`
			}
			err := app.DB().NewQuery("SELECT LOWER([[email]]) AS [[email]] FROM {{" + collection.Name + "}} WHERE [[email]] != '' GROUP BY LOWER([[email]]) HAVING COUNT(*) > 1").All(&dupes)
			if err != nil {
				return err
			}
			if len(dupes) > 0 {
				emails := make([]string, len(dupes))
				for i, d := range dupes {
					emails[i] = d.Email
				}
				app.Logger().Warn("Skipping case-insensitive email index: case-variant duplicate emails exist",
					"collection", collection.Name, "emails", strings.Join(emails, ", "))
				continue
			}
		}

		changed := false
		for i, raw := range collection.Indexes {
			idx := dbutils.ParseIndex(raw)
			if !idx.Unique || len(idx.Columns) != 1 || !strings.EqualFold(idx.Columns[0].Name, core.FieldNameEmail) {
				continue
			}
			if strings.EqualFold(idx.Columns[0].Collate, collate) {
				continue
			}
			idx.Columns[0].Collate = collate
			collection.Indexes[i] = idx.Build()
			changed = true
		}
		if !changed {
			continue
		}
		if err := app.Save(collection); err != nil {
			return err
		}
	}
	return nil
}
