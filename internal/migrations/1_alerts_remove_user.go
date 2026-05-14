package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		db := app.DB()

		// Deduplicate alerts before dropping the user column:
		// for each (system, name) pair, keep the most recently updated record.
		_, err := db.NewQuery(`
			DELETE FROM alerts
			WHERE id NOT IN (
				SELECT id FROM (
					SELECT id, ROW_NUMBER() OVER (PARTITION BY system, name ORDER BY updated DESC) AS rn
					FROM alerts
				) ranked
				WHERE rn = 1
			)
		`).Execute()
		if err != nil {
			return err
		}

		// Drop old unique index (user, system, name) before modifying alerts schema
		_, err = db.NewQuery("DROP INDEX IF EXISTS `idx_MnhEt21L5r`").Execute()
		if err != nil {
			return err
		}

		// Drop user index on alerts_history
		_, err = db.NewQuery("DROP INDEX IF EXISTS `idx_YdGnup5aqB`").Execute()
		if err != nil {
			return err
		}

		// Update alerts collection: remove user field, update index and rules
		alertsCollection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		alertsCollection.Fields.RemoveById("hn5ly3vi") // user field
		alertsCollection.Indexes = []string{
			"CREATE UNIQUE INDEX `idx_MnhEt21L5r` ON `alerts` (`system`, `name`)",
		}
		listRule := "@request.auth.id != \"\""
		adminRule := "@request.auth.role = 'admin'"
		alertsCollection.ListRule = &listRule
		alertsCollection.ViewRule = &listRule
		alertsCollection.CreateRule = &adminRule
		alertsCollection.UpdateRule = &adminRule
		alertsCollection.DeleteRule = &adminRule
		if err := app.Save(alertsCollection); err != nil {
			return err
		}

		// Update alerts_history collection: remove user field, update rules
		alertsHistoryCollection, err := app.FindCollectionByNameOrId("alerts_history")
		if err != nil {
			return err
		}
		alertsHistoryCollection.Fields.RemoveById("relation2375276105") // user field
		alertsHistoryCollection.Indexes = []string{
			"CREATE INDEX `idx_taLet9VdME` ON `alerts_history` (`created`)",
		}
		alertsHistoryCollection.ListRule = &listRule
		alertsHistoryCollection.ViewRule = &listRule
		alertsHistoryCollection.CreateRule = nil
		alertsHistoryCollection.UpdateRule = nil
		alertsHistoryCollection.DeleteRule = nil
		if err := app.Save(alertsHistoryCollection); err != nil {
			return err
		}

		return nil
	}, nil)
}
