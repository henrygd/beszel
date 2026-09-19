package transport

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWiFiSequentialResponseSnapshots(t *testing.T) {
	signal := -50.0
	var decoded system.CombinedData
	for _, snapshot := range []map[string]system.WiFi{
		{"wlan0": {SSID: "home", Signal: &signal}, "wlan1": {Signal: &signal}},
		{"wlan0": {SSID: "home"}}, {}, nil,
		{"wlan1": {SSID: "new", Signal: &signal}},
	} {
		payload, err := cbor.Marshal(system.CombinedData{Info: system.Info{WiFi: snapshot}, Stats: system.Stats{WiFi: snapshot}})
		require.NoError(t, err)
		require.NoError(t, UnmarshalResponse(common.AgentResponse{Data: payload}, common.GetData, &decoded))
		require.Len(t, decoded.Info.WiFi, len(snapshot))
		require.Len(t, decoded.Stats.WiFi, len(snapshot))
		for id, want := range snapshot {
			require.Equal(t, want, decoded.Info.WiFi[id])
			require.Equal(t, want, decoded.Stats.WiFi[id])
		}
	}
	// An older generic-response agent may omit both fields entirely.
	payload, err := cbor.Marshal(map[int]any{0: map[int]any{}, 1: map[int]any{}})
	require.NoError(t, err)
	require.NoError(t, UnmarshalResponse(common.AgentResponse{Data: payload}, common.GetData, &decoded))
	require.Empty(t, decoded.Info.WiFi)
	require.Empty(t, decoded.Stats.WiFi)
}
