package system

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestWiFiWireSnapshot(t *testing.T) {
	signal := -55.0
	for _, wifi := range []map[string]WiFi{nil, {}, {"wlan0": {SSID: "home", Signal: &signal}, "wlan1": {}}} {
		original := CombinedData{Info: Info{WiFi: wifi}, Stats: Stats{WiFi: make(map[string]int8, len(wifi))}}
		for id := range wifi {
			original.Stats.WiFi[id] = -55
		}
		encoded, err := cbor.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var decoded CombinedData
		if err = cbor.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded.Info.WiFi) != len(wifi) || len(decoded.Stats.WiFi) != len(wifi) {
			t.Fatal(decoded)
		}
		encoded, err = json.Marshal(decoded.Info)
		if err != nil {
			t.Fatal(err)
		}
		var info map[string]any
		if err = json.Unmarshal(encoded, &info); err != nil {
			t.Fatal(err)
		}
		if _, ok := info["wifi"]; !ok {
			t.Fatal("current absence must be explicit")
		}
	}
}
