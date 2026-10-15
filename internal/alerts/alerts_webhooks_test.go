//go:build testing

package alerts_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/require"
)

func TestPersistedWebhooksUseCurrentOwnerRole(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	var delivered atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
	}))
	defer server.Close()

	settings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", dbx.Params{"user": user.Id})
	require.NoError(t, err)
	settings.Set("settings", alerts.UserNotificationSettings{Webhooks: []string{"generic+" + server.URL}})
	require.NoError(t, hub.Save(settings))
	message := alerts.AlertMessageData{UserID: user.Id, Title: "Test", Message: "Persisted webhook"}

	// Keep the same URL and manager while changing roles, so cached privileges
	// or treating previously saved URLs as trusted would fail this test.
	for _, tc := range []struct {
		name string
		role string
		want int32
	}{
		{"regular user", "user", 0},
		{"readonly user", "readonly", 0},
		{"promoted admin", "admin", 1},
		{"demoted admin", "user", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user.Set("role", tc.role)
			require.NoError(t, hub.Save(user))
			// Webhook errors are logged; SendAlert continues to email delivery.
			require.NoError(t, am.SendAlert(message))
			require.Equal(t, tc.want, delivered.Load())
		})
	}

	t.Run("missing owner fails closed", func(t *testing.T) {
		// Model an orphaned settings record without deleting it through the
		// normal user deletion cascade.
		const missingOwner = "missingowner123"
		settings.Set("user", missingOwner)
		require.NoError(t, hub.SaveNoValidate(settings))
		message.UserID = missingOwner
		err := am.SendAlert(message)
		require.ErrorContains(t, err, "load notification owner")
		require.EqualValues(t, 1, delivered.Load())
	})
}
