package migrations

import (
	"encoding/json"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		db := app.DB()

		// Backfill per-user notification preferences before the user column is
		// dropped. Every user who had alerts is opted in to notifications and
		// subscribed to exactly the systems they had alerts on, so notification
		// behavior is preserved across the upgrade. Runs before the dedup delete
		// below so a user's subscription isn't lost when another user's alert
		// wins the (system, name) tie-break.
		type alertUserSystem struct {
			User   string `db:"user"`
			System string `db:"system"`
		}
		var pairs []alertUserSystem
		if err := db.NewQuery(
			"SELECT DISTINCT user, system FROM alerts WHERE user != '' AND system != ''",
		).All(&pairs); err != nil {
			return err
		}
		systemsByUser := make(map[string][]string)
		for _, p := range pairs {
			systemsByUser[p.User] = append(systemsByUser[p.User], p.System)
		}
		for userID, systemIDs := range systemsByUser {
			settingsRec, err := app.FindFirstRecordByFilter(
				"user_settings", "user={:user}", dbx.Params{"user": userID},
			)
			if err != nil {
				continue // user has no settings record; nothing to migrate
			}
			settings := map[string]any{}
			if raw := settingsRec.GetString("settings"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &settings); err != nil {
					return err
				}
			}
			settings["notificationsEnabled"] = true
			settings["systems"] = systemIDs
			settingsRec.Set("settings", settings)
			if err := app.SaveNoValidate(settingsRec); err != nil {
				return err
			}
		}

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
