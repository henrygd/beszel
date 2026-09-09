//go:build testing

package records_test

import (
	"os"
	"testing"

	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRetentionDurationPrecedence(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	// default without env or DB change should be 30d
	t.Setenv("BESZEL_HUB_RETENTION", "")
	t.Setenv("RETENTION", "")
	os.Unsetenv("BESZEL_HUB_RETENTION")
	os.Unsetenv("RETENTION")
	rec, err := hub.FindRecordById("hub_settings", "hubsettings0001")
	require.NoError(t, err)
	rec.Set("retention", "30d")
	require.NoError(t, hub.Save(rec))
	assert.Equal(t, "30d", records.GetRetentionString(hub))
	assert.NotZero(t, records.GetRetentionDuration(hub))

	// env overrides DB
	t.Setenv("BESZEL_HUB_RETENTION", "365d")
	assert.Equal(t, "365d", records.GetRetentionString(hub))
	assert.Equal(t, 365*24*60*60, int(records.GetRetentionDuration(hub).Seconds()))
	assert.True(t, records.IsEnvOverride())

	// invalid env falls back to DB
	t.Setenv("BESZEL_HUB_RETENTION", "invalid")
	assert.Equal(t, "30d", records.GetRetentionString(hub))
	assert.False(t, records.IsEnvOverride())

	// valid unprefixed RETENTION fallback when prefixed not set
	t.Setenv("BESZEL_HUB_RETENTION", "")
	os.Unsetenv("BESZEL_HUB_RETENTION")
	t.Setenv("RETENTION", "730d")
	assert.Equal(t, "730d", records.GetRetentionString(hub))
	assert.True(t, records.IsEnvOverride())
	t.Setenv("RETENTION", "")
	os.Unsetenv("RETENTION")
	os.Unsetenv("BESZEL_HUB_RETENTION")

	// DB invalid falls back to 30d - use SaveNoValidate to bypass select validation
	rec.Set("retention", "bogus")
	require.NoError(t, hub.SaveNoValidate(rec))
	assert.Equal(t, "30d", records.GetRetentionString(hub))

	// never => 0 duration
	rec.Set("retention", "never")
	require.NoError(t, hub.Save(rec))
	assert.Equal(t, "never", records.GetRetentionString(hub))
	assert.Equal(t, 0, int(records.GetRetentionDuration(hub).Seconds()))

	// env never overrides
	t.Setenv("BESZEL_HUB_RETENTION", "never")
	assert.Equal(t, "never", records.GetRetentionString(hub))
	assert.Equal(t, 0, int(records.GetRetentionDuration(hub).Seconds()))
	assert.True(t, records.IsEnvOverride())
	t.Setenv("BESZEL_HUB_RETENTION", "")
	os.Unsetenv("BESZEL_HUB_RETENTION")
}

func TestEnsureHubSettingsSingleton(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	col, err := hub.FindCollectionByNameOrId("hub_settings")
	require.NoError(t, err)
	extra := core.NewRecord(col)
	extra.Set("id", "hubsettings0002")
	extra.Set("retention", "365d")
	require.NoError(t, hub.Save(extra))

	count, err := hub.CountRecords("hub_settings")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	require.NoError(t, records.EnsureHubSettingsExists(hub))
	count, err = hub.CountRecords("hub_settings")
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestDeleteOldSystemStatsBatched(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	// ensure retention 30d
	rec, err := hub.FindRecordById("hub_settings", "hubsettings0001")
	require.NoError(t, err)
	rec.Set("retention", "30d")
	require.NoError(t, hub.Save(rec))

	// create system
	user, err := tests.CreateUser(hub, "batch@test.com", "password123")
	require.NoError(t, err)
	sys, err := tests.CreateRecord(hub, "systems", map[string]any{"name": "batch-sys", "host": "127.0.0.1", "users": []string{user.Id}, "status": "up"})
	require.NoError(t, err)

	// create 1500 old 480m records (45 days old, beyond 30d) - should be deleted in batches of 1000
	for i := 0; i < 1500; i++ {
		r, err := tests.CreateRecord(hub, "system_stats", map[string]any{"system": sys.Id, "type": "480m", "stats": `{"cpu":1}`})
		require.NoError(t, err)
		// set created to 45 days ago via SaveNoValidate with raw created
		r.SetRaw("created", "2020-01-01 00:00:00.000Z")
		require.NoError(t, hub.SaveNoValidate(r))
	}
	// also create 10 recent ones that should be kept
	for i := 0; i < 10; i++ {
		_, err := tests.CreateRecord(hub, "system_stats", map[string]any{"system": sys.Id, "type": "480m", "stats": `{"cpu":1}`})
		require.NoError(t, err)
	}

	countBefore, err := hub.CountRecords("system_stats")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, countBefore, int64(1510))

	// run batched delete
	require.NoError(t, records.DeleteOldSystemStats(hub))

	countAfter, err := hub.CountRecords("system_stats")
	require.NoError(t, err)
	// should have deleted 1500 old, kept 10 recent
	assert.Equal(t, int64(10), countAfter)
}
