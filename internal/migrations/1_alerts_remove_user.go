package migrations

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		db := app.DB()

		alertsCollection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		quietHoursCollection, err := app.FindCollectionByNameOrId("quiet_hours")
		if err != nil {
			return err
		}

		listRule := "@request.auth.id != \"\""
		adminRule := "@request.auth.role = 'admin'"

		// Notification channels are shared destinations managed by admins.
		// Alerts pick the channels they are delivered to.
		channelsCollection := core.NewBaseCollection("notification_channels")
		channelsCollection.ListRule = &adminRule
		channelsCollection.ViewRule = &adminRule
		channelsCollection.CreateRule = &adminRule
		channelsCollection.UpdateRule = &adminRule
		channelsCollection.DeleteRule = &adminRule
		channelsCollection.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 100},
			&core.JSONField{Name: "emails"},
			&core.JSONField{Name: "webhooks"},
			&core.BoolField{Name: "default"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		if err := app.Save(channelsCollection); err != nil {
			return err
		}

		alertsCollection.Fields.Add(&core.RelationField{
			Name:         "channels",
			CollectionId: channelsCollection.Id,
			MaxSelect:    99,
		})
		if err := app.Save(alertsCollection); err != nil {
			return err
		}
		quietHoursCollection.Fields.Add(&core.RelationField{
			Name:          "channel",
			CollectionId:  channelsCollection.Id,
			MaxSelect:     1,
			CascadeDelete: true,
		})
		if err := app.Save(quietHoursCollection); err != nil {
			return err
		}

		// Record which alerts each user owned before the user column is dropped,
		// so their channel can be attached to the alert that survives the
		// (system, name) deduplication below.
		type alertKey struct {
			User   string `db:"user"`
			System string `db:"system"`
			Name   string `db:"name"`
		}
		alertKeysByUser := make(map[string][]alertKey)
		if alertsCollection.Fields.GetByName("user") != nil {
			var keys []alertKey
			if err := db.NewQuery(
				"SELECT DISTINCT user, system, name FROM alerts WHERE user != '' AND system != ''",
			).All(&keys); err != nil {
				return err
			}
			for _, k := range keys {
				alertKeysByUser[k.User] = append(alertKeysByUser[k.User], k)
			}
		}

		// Deduplicate alerts before dropping the user column:
		// for each (system, name) pair, keep the most recently updated record.
		var duplicateIDs []string
		if err := db.NewQuery(`
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY system, name ORDER BY updated DESC) AS rn
				FROM alerts
			) ranked
			WHERE rn > 1
		`).Column(&duplicateIDs); err != nil {
			return err
		}
		if len(duplicateIDs) > 0 {
			ids := make([]any, len(duplicateIDs))
			for i, id := range duplicateIDs {
				ids[i] = id
			}
			// Raw deletes skip the record hooks, so resolve open history entries
			// of the removed alerts here the same way the delete hook would.
			if _, err := db.Update("alerts_history",
				dbx.Params{"resolved": types.NowDateTime().String()},
				dbx.And(dbx.In("alert_id", ids...), dbx.NewExp("(resolved IS NULL OR resolved = '')")),
			).Execute(); err != nil {
				return err
			}
			if _, err := db.Delete("alerts", dbx.In("id", ids...)).Execute(); err != nil {
				return err
			}
		}

		// Turn each user's notification settings into channels attached to the
		// alerts they owned. Webhooks are only carried over for admins: other
		// users' webhooks were restricted to public destinations, and channels
		// are sent without that restriction. Admin channels become defaults so
		// they keep receiving automatic (SMART, ZFS) and unassigned alerts.
		settingsRecords, err := app.FindAllRecords("user_settings")
		if err != nil {
			return err
		}
		usedNames := make(map[string]bool)
		for _, settingsRec := range settingsRecords {
			userID := settingsRec.GetString("user")
			settings := map[string]any{}
			if raw := settingsRec.GetString("settings"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &settings); err != nil {
					return err
				}
			}
			var notify struct {
				Emails   []string `json:"emails"`
				Webhooks []string `json:"webhooks"`
			}
			if err := settingsRec.UnmarshalJSONField("settings", &notify); err != nil {
				return err
			}
			for _, key := range []string{"emails", "webhooks", "notificationsEnabled", "systems"} {
				delete(settings, key)
			}
			settingsRec.Set("settings", settings)
			if err := app.SaveNoValidate(settingsRec); err != nil {
				return err
			}

			user, err := app.FindRecordById("users", userID)
			if err != nil {
				continue
			}
			isAdmin := user.GetString("role") == "admin"
			webhooks := notify.Webhooks
			if !isAdmin {
				if len(webhooks) > 0 {
					app.Logger().Warn("Skipped migrating webhooks of non-admin user to a notification channel", "user", user.GetString("email"), "count", len(webhooks))
				}
				webhooks = nil
			}
			keys := alertKeysByUser[userID]
			if len(notify.Emails) == 0 && len(webhooks) == 0 || !isAdmin && len(keys) == 0 {
				continue
			}

			// One channel per destination: all emails together, and one per
			// webhook named after its service, so alerts can be routed to each
			// destination separately.
			type channelSpec struct {
				name     string
				emails   []string
				webhooks []string
			}
			var specs []channelSpec
			if len(notify.Emails) > 0 {
				specs = append(specs, channelSpec{name: "Email", emails: notify.Emails})
			}
			for _, webhook := range webhooks {
				specs = append(specs, channelSpec{name: webhookServiceName(webhook), webhooks: []string{webhook}})
			}

			channelIDs := make([]string, 0, len(specs))
			for _, spec := range specs {
				channel := core.NewRecord(channelsCollection)
				channel.Set("name", uniqueChannelName(usedNames, spec.name, user.GetString("email")))
				channel.Set("emails", nonNil(spec.emails))
				channel.Set("webhooks", nonNil(spec.webhooks))
				channel.Set("default", isAdmin)
				if err := app.SaveNoValidate(channel); err != nil {
					return err
				}
				channelIDs = append(channelIDs, channel.Id)
			}

			for _, k := range keys {
				alert, err := app.FindFirstRecordByFilter("alerts",
					"system={:system} && name={:name}",
					dbx.Params{"system": k.System, "name": k.Name})
				if err != nil {
					continue
				}
				alert.Set("channels", append(alert.GetStringSlice("channels"), channelIDs...))
				if err := app.SaveNoValidate(alert); err != nil {
					return err
				}
			}

			// The user's quiet hours apply to each of their channels.
			if quietHoursCollection.Fields.GetByName("user") != nil {
				windows, err := app.FindAllRecords("quiet_hours", dbx.HashExp{"user": userID})
				if err != nil {
					return err
				}
				for _, window := range windows {
					for i, channelID := range channelIDs {
						target := window
						if i > 0 {
							target = core.NewRecord(quietHoursCollection)
							for _, field := range quietHoursCollection.Fields {
								switch field.GetName() {
								case "id", "created", "updated":
									continue
								}
								target.Set(field.GetName(), window.Get(field.GetName()))
							}
						}
						target.Set("channel", channelID)
						if err := app.SaveNoValidate(target); err != nil {
							return err
						}
					}
				}
			}
		}

		// Quiet hours move from users to channels. Windows of users without a
		// channel are dropped; an empty channel would apply to all channels.
		if quietHoursCollection.Fields.GetByName("user") != nil {
			if _, err := db.Delete("quiet_hours", dbx.NewExp("channel = '' OR channel IS NULL")).Execute(); err != nil {
				return err
			}
			if _, err := db.NewQuery("DROP INDEX IF EXISTS `idx_q0iKnRP9v8`").Execute(); err != nil {
				return err
			}
			quietHoursCollection.Fields.RemoveByName("user")
			quietHoursCollection.Indexes = []string{
				"CREATE INDEX `idx_q0iKnRP9v8` ON `quiet_hours` (`channel`, `system`)",
				"CREATE INDEX `idx_6T7ljT7FJd` ON `quiet_hours` (`type`, `end`)",
			}
			quietHoursCollection.ListRule = &adminRule
			quietHoursCollection.ViewRule = &adminRule
			quietHoursCollection.CreateRule = &adminRule
			quietHoursCollection.UpdateRule = &adminRule
			quietHoursCollection.DeleteRule = &adminRule
			if err := app.Save(quietHoursCollection); err != nil {
				return err
			}
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
		alertsCollection.Fields.RemoveById("hn5ly3vi") // user field
		alertsCollection.Indexes = []string{
			"CREATE UNIQUE INDEX `idx_MnhEt21L5r` ON `alerts` (`system`, `name`)",
		}
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

// webhookServiceName derives a channel name from a shoutrrr URL scheme,
// e.g. "discord://..." -> "Discord", "generic+https://..." -> "Webhook".
func webhookServiceName(webhook string) string {
	scheme, _, found := strings.Cut(webhook, "://")
	if !found || scheme == "" {
		return "Webhook"
	}
	scheme, _, _ = strings.Cut(strings.ToLower(scheme), "+")
	switch scheme {
	case "generic":
		return "Webhook"
	case "ntfy":
		return "ntfy"
	case "smtp":
		return "SMTP"
	case "teams":
		return "Microsoft Teams"
	case "googlechat":
		return "Google Chat"
	}
	return strings.ToUpper(scheme[:1]) + scheme[1:]
}

// uniqueChannelName returns name if unused, otherwise qualifies it with the
// owner's email and then a counter, and marks the result as used.
func uniqueChannelName(used map[string]bool, name, owner string) string {
	candidate := name
	if used[candidate] && owner != "" {
		candidate = fmt.Sprintf("%s (%s)", name, owner)
	}
	base := candidate
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s %d", base, i)
	}
	used[candidate] = true
	return candidate
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
