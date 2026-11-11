//go:build testing

package alerts_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestSendAlertRoutesToChannels(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	fallback := defaultChannel(t, hub)
	other, err := beszelTests.CreateChannel(hub, "other", "other@example.com", false)
	require.NoError(t, err)

	recipients := func() []string {
		var to []string
		for _, msg := range hub.TestMailer.Messages() {
			for _, addr := range msg.To {
				to = append(to, addr.Address)
			}
		}
		hub.TestMailer.Reset()
		return to
	}

	t.Run("no channels uses defaults", func(t *testing.T) {
		require.NoError(t, am.SendAlert(alerts.AlertMessageData{Title: "Test"}))
		require.Equal(t, []string{"test@example.com"}, recipients())
	})

	t.Run("selected channels only", func(t *testing.T) {
		require.NoError(t, am.SendAlert(alerts.AlertMessageData{Title: "Test", Channels: []string{other.Id}}))
		require.Equal(t, []string{"other@example.com"}, recipients())
	})

	t.Run("multiple selected channels", func(t *testing.T) {
		require.NoError(t, am.SendAlert(alerts.AlertMessageData{Title: "Test", Channels: []string{fallback.Id, other.Id}}))
		require.ElementsMatch(t, []string{"test@example.com", "other@example.com"}, recipients())
	})

	t.Run("deleted channel is removed from alerts", func(t *testing.T) {
		systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
		require.NoError(t, err)
		alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name": "CPU", "system": systems[0].Id, "value": 80, "min": 1, "channels": []string{other.Id},
		})
		require.NoError(t, err)
		require.NoError(t, hub.Delete(other))
		alert, err = hub.FindRecordById("alerts", alert.Id)
		require.NoError(t, err)
		require.Empty(t, alert.GetStringSlice("channels"))
	})
}

func TestSendAlertChannelWebhooksReachInternalServices(t *testing.T) {
	hub, _ := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	var delivered atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
	}))
	defer server.Close()

	// Channels are managed by admins, so loopback destinations are allowed.
	channel := defaultChannel(t, hub)
	channel.Set("webhooks", []string{"generic+" + server.URL})
	require.NoError(t, hub.Save(channel))

	require.NoError(t, am.SendAlert(alerts.AlertMessageData{Title: "Test", Message: "Channel webhook"}))
	require.EqualValues(t, 1, delivered.Load())
}
