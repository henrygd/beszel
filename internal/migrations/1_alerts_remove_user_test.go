//go:build testing

package migrations_test

import (
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restoreUserColumns re-adds the per-user alert schema that the migration removes.
func restoreUserColumns(t *testing.T, app core.App) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	alerts, err := app.FindCollectionByNameOrId("alerts")
	require.NoError(t, err)
	alerts.Fields.Add(&core.RelationField{Id: "hn5ly3vi", Name: "user", CollectionId: users.Id, MaxSelect: 1, CascadeDelete: true})
	alerts.Indexes = []string{"CREATE UNIQUE INDEX `idx_MnhEt21L5r` ON `alerts` (`user`, `system`, `name`)"}
	require.NoError(t, app.Save(alerts))

	history, err := app.FindCollectionByNameOrId("alerts_history")
	require.NoError(t, err)
	history.Fields.Add(&core.RelationField{Id: "relation2375276105", Name: "user", CollectionId: users.Id, MaxSelect: 1, CascadeDelete: true})
	require.NoError(t, app.Save(history))
}

func runAlertsRemoveUserMigration(t *testing.T, app core.App) {
	t.Helper()
	for _, migration := range core.AppMigrations.Items() {
		if migration.File == "1_alerts_remove_user.go" {
			require.NoError(t, app.RunInTransaction(migration.Up))
			return
		}
	}
	t.Fatal("migration 1_alerts_remove_user.go not registered")
}

func TestAlertsRemoveUserResolvesHistoryOfMergedDuplicates(t *testing.T) {
	hub, user1 := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()
	user2, err := beszelTests.CreateUser(hub, "user2@example.com", "testtesttest")
	require.NoError(t, err)
	for _, user := range []*core.Record{user1, user2} {
		if _, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", dbx.Params{"user": user.Id}); err != nil {
			_, err = beszelTests.CreateRecord(hub, "user_settings", map[string]any{"user": user.Id, "settings": map[string]any{}})
			require.NoError(t, err)
		}
	}
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name": "test-system", "host": "localhost", "port": "45876", "status": "up",
		"users": []string{user1.Id, user2.Id},
	})
	require.NoError(t, err)

	restoreUserColumns(t, hub)

	// Both users have a triggered CPU alert on the same system; user2's is newer and wins.
	older, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"user": user1.Id, "system": system.Id, "name": "CPU", "value": 80, "min": 1, "triggered": true,
	})
	require.NoError(t, err)
	newer, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"user": user2.Id, "system": system.Id, "name": "CPU", "value": 90, "min": 1, "triggered": true,
	})
	require.NoError(t, err)
	_, err = hub.DB().Update("alerts", dbx.Params{"updated": "2020-01-01 00:00:00.000Z"}, dbx.HashExp{"id": older.Id}).Execute()
	require.NoError(t, err)

	openHistory := func(alertID, userID string) *core.Record {
		record, err := beszelTests.CreateRecord(hub, "alerts_history", map[string]any{
			"alert_id": alertID, "user": userID, "system": system.Id, "name": "CPU", "value": 95,
		})
		require.NoError(t, err)
		return record
	}
	olderOpen := openHistory(older.Id, user1.Id)
	newerOpen := openHistory(newer.Id, user2.Id)
	olderResolved := openHistory(older.Id, user1.Id)
	olderResolved.Set("resolved", "2021-01-01 00:00:00.000Z")
	require.NoError(t, hub.Save(olderResolved))

	runAlertsRemoveUserMigration(t, hub)

	_, err = hub.FindRecordById("alerts", older.Id)
	assert.Error(t, err, "older duplicate should be deleted")
	_, err = hub.FindRecordById("alerts", newer.Id)
	assert.NoError(t, err, "most recently updated duplicate should be kept")

	olderOpen, err = hub.FindRecordById("alerts_history", olderOpen.Id)
	require.NoError(t, err)
	assert.False(t, olderOpen.GetDateTime("resolved").IsZero(), "open history of deleted duplicate should be resolved")

	newerOpen, err = hub.FindRecordById("alerts_history", newerOpen.Id)
	require.NoError(t, err)
	assert.True(t, newerOpen.GetDateTime("resolved").IsZero(), "open history of kept alert should stay open")

	olderResolved, err = hub.FindRecordById("alerts_history", olderResolved.Id)
	require.NoError(t, err)
	assert.Equal(t, "2021-01-01 00:00:00.000Z", olderResolved.GetDateTime("resolved").String(), "already resolved history should be untouched")

	// Both users stay subscribed to the system they had alerts on.
	for _, userID := range []string{user1.Id, user2.Id} {
		settings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", dbx.Params{"user": userID})
		require.NoError(t, err)
		var parsed struct {
			NotificationsEnabled bool     `json:"notificationsEnabled"`
			Systems              []string `json:"systems"`
		}
		require.NoError(t, settings.UnmarshalJSONField("settings", &parsed))
		assert.True(t, parsed.NotificationsEnabled)
		assert.Equal(t, []string{system.Id}, parsed.Systems)
	}
}
