//go:build testing

package systems

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/require"
)

func TestCreateRecordsWiFiDisconnectReconnect(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	signal := -50.0
	for _, snapshot := range []map[string]system.WiFi{
		{"wlan0": {SSID: "home", Signal: &signal}, "wlan1": {SSID: "other"}},
		{}, nil,
		{"wlan0": {SSID: "new", Signal: &signal}},
	} {
		_, err := sys.createRecords(&system.CombinedData{Info: system.Info{WiFi: snapshot}})
		require.NoError(t, err)
		record, err := app.FindRecordById("systems", sys.Id)
		require.NoError(t, err)
		var info system.Info
		require.NoError(t, record.UnmarshalJSONField("info", &info))
		require.Len(t, info.WiFi, len(snapshot), "current info must replace previous connection state")
	}
}
