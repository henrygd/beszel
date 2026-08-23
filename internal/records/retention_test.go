//go:build testing

package records_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"
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
	// ensure hub_settings has 30d
	rec, err := hub.FindRecordById("hub_settings", "hubsettings0001")
	require.NoError(t, err)
	rec.Set("retention", "30d")
	require.NoError(t, hub.Save(rec))
	assert.Equal(t, "30d", records.GetRetentionString(hub))
	assert.NotZero(t, records.GetRetentionDuration(hub))

	// env overrides DB
	t.Setenv("BESZEL_HUB_RETENTION", "365d")
	// DB still 30d but effective should be 365d
	assert.Equal(t, "365d", records.GetRetentionString(hub))
	assert.Equal(t, 365*24*60*60, int(records.GetRetentionDuration(hub).Seconds()))
	assert.True(t, records.IsEnvOverride())

	// invalid env falls back to DB
	t.Setenv("BESZEL_HUB_RETENTION", "invalid")
	assert.Equal(t, "30d", records.GetRetentionString(hub))
	assert.False(t, records.IsEnvOverride() && false) // invalid not considered override; check fallback
	// IsEnvOverride should be false for invalid
	assert.False(t, records.IsEnvOverride())

	// valid RETENTION fallback (unprefixed)
	t.Setenv("BESZEL_HUB_RETENTION", "")
	t.Setenv("RETENTION", "730d")
	assert.Equal(t, "730d", records.GetRetentionString(hub))
	assert.True(t, records.IsEnvOverride())
	t.Setenv("RETENTION", "")

	// DB invalid falls back to 30d
	rec.Set("retention", "bogus")
	require.NoError(t, hub.Save(rec))
	t.Setenv("BESZEL_HUB_RETENTION", "")
	t.Setenv("RETENTION", "")
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
}

func TestEnsureHubSettingsSingleton(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	// create extra row with different id
	col, err := hub.FindCollectionByNameOrId("hub_settings")
	require.NoError(t, err)
	extra := col.NewRecord()
	extra.Set("id", "hubsettings0002")
	extra.Set("retention", "365d")
	require.NoError(t, hub.Save(extra))

	count, err := hub.CountRecords("hub_settings")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Ensure should clean up to 1
	require.NoError(t, records.EnsureHubSettingsExists(hub))
	count, err = hub.CountRecords("hub_settings")
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}
