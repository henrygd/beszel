//go:build testing

package users_test

import (
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/stretchr/testify/require"
)

func TestInitializeUserSettingsDefaults(t *testing.T) {
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	require.NoError(t, hub.StartHub())

	user, err := beszelTests.CreateUser(hub, "test@example.com", "password")
	require.NoError(t, err)

	record, err := beszelTests.CreateRecord(hub, "user_settings", map[string]any{
		"user": user.Id,
	})
	require.NoError(t, err)

	settings := struct {
		ChartTime      string   `json:"chartTime"`
		MaxChartPeriod string   `json:"maxChartPeriod"`
		Emails         []string `json:"emails"`
	}{}
	require.NoError(t, record.UnmarshalJSONField("settings", &settings))

	require.Equal(t, "1h", settings.ChartTime)
	require.Equal(t, "30d", settings.MaxChartPeriod)
	require.Equal(t, []string{"test@example.com"}, settings.Emails)
}
