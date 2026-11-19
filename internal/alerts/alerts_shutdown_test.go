//go:build testing

package alerts_test

import (
	"sync"
	"testing"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestStandaloneAlertManagerStopsBeforeDatabaseCleanup(t *testing.T) {
	for _, state := range []string{"pending", "already stopped", "delivered"} {
		t.Run(state, func(t *testing.T) {
			hub, user := beszelTests.GetHubWithUser(t)
			cleanup := sync.OnceFunc(hub.Cleanup)
			defer cleanup()
			setStatusAlertEmail(t, hub, user.Id, "shutdown@example.com")
			systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
			require.NoError(t, err)
			_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
				"name": "Status", "system": systems[0].Id, "user": user.Id, "min": 1,
			})
			require.NoError(t, err)

			am := alerts.NewTestAlertManagerWithoutWorker(hub)
			defer am.Stop()
			require.NoError(t, am.HandleStatusAlerts("down", systems[0]))
			require.Equal(t, 1, am.GetPendingAlertsCount())
			switch state {
			case "already stopped":
				am.Stop()
			case "delivered":
				am.ForceExpirePendingAlerts()
				processed, err := am.ProcessPendingAlerts()
				require.NoError(t, err)
				require.Len(t, processed, 1)
				require.Equal(t, 1, hub.TestMailer.TotalSend())
			}

			cleanup()
			// No timer may retain the disposed app until its eventual deadline.
			require.Zero(t, am.GetPendingAlertsCount())
		})
	}
}
