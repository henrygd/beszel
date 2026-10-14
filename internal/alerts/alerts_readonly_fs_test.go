//go:build testing

package alerts_test

import (
	"testing"
	"testing/synctest"

	"github.com/henrygd/beszel/internal/entities/system"
	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilesystemReadOnlyAlert(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
	require.NoError(t, err)
	systemRecord := systems[0]

	alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "FilesystemReadOnly",
		"system": systemRecord.Id,
		"user":   user.Id,
		"value":  0,
		"min":    0,
	})
	require.NoError(t, err)
	assert.False(t, alert.GetBool("triggered"))

	am := hub.GetAlertManager()

	synctest.Test(t, func(t *testing.T) {
		require.NoError(t, am.HandleSystemAlerts(systemRecord, &system.CombinedData{
			Info: system.Info{RootReadOnly: true, ReadOnlyFilesystems: []string{"root"}},
		}))
	})

	alert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alert.Id})
	require.NoError(t, err)
	assert.True(t, alert.GetBool("triggered"), "Alert should trigger when root is read-only")
	assert.Equal(t, 1, hub.TestMailer.TotalSend())

	synctest.Test(t, func(t *testing.T) {
		require.NoError(t, am.HandleSystemAlerts(systemRecord, &system.CombinedData{
			Info: system.Info{RootReadOnly: true, ReadOnlyFilesystems: []string{"root"}},
		}))
	})
	assert.Equal(t, 1, hub.TestMailer.TotalSend(), "Should not re-notify while still triggered")

	synctest.Test(t, func(t *testing.T) {
		require.NoError(t, am.HandleSystemAlerts(systemRecord, &system.CombinedData{
			Info: system.Info{},
		}))
	})

	alert, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alert.Id})
	require.NoError(t, err)
	assert.False(t, alert.GetBool("triggered"), "Alert should resolve when filesystems are writable")
	assert.Equal(t, 2, hub.TestMailer.TotalSend())
}
