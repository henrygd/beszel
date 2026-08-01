//go:build testing

package systems_test

import (
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/systemd"
	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createSystemdStatsRecords upserts the reported services and must drop rows for
// services the agent has stopped reporting, so a unit removed from the host doesn't
// linger with its last known state until the retention sweep.
func TestCreateSystemdStatsRecordsRemovesUnreportedServices(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	user, err := tests.CreateUser(hub, "test@example.com", "password")
	require.NoError(t, err)
	system, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"host":  "127.0.0.1",
		"users": []string{user.Id},
	})
	require.NoError(t, err)

	serviceNames := func() []string {
		var out []string
		require.NoError(t, hub.DB().Select("name").From("systemd_services").
			Where(dbx.NewExp("system={:s}", dbx.Params{"s": system.Id})).
			OrderBy("name").Column(&out))
		return out
	}

	require.NoError(t, systems.CreateSystemdStatsRecords(hub, []*systemd.Service{
		{Name: "a.service", State: systemd.StatusActive},
		{Name: "gone.service", State: systemd.StatusFailed},
	}, system.Id))
	assert.Equal(t, []string{"a.service", "gone.service"}, serviceNames())

	// Batches are stamped with millisecond precision and update cycles are a minute
	// apart in practice; ensure the next batch gets a distinct timestamp.
	time.Sleep(2 * time.Millisecond)

	// gone.service is no longer reported, so its row must not survive.
	require.NoError(t, systems.CreateSystemdStatsRecords(hub, []*systemd.Service{
		{Name: "a.service", State: systemd.StatusActive},
	}, system.Id))
	assert.Equal(t, []string{"a.service"}, serviceNames())

	// An empty payload is a no-op and must not clear the table.
	require.NoError(t, systems.CreateSystemdStatsRecords(hub, nil, system.Id))
	assert.Equal(t, []string{"a.service"}, serviceNames())
}
