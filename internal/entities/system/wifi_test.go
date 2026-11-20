package system

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestWiFiWireSnapshot(t *testing.T) {
	signal := -55.0
	for _, wifi := range []map[string]WiFi{nil, {}, {"wlan0": {SSID: "home", Signal: &signal}, "wlan1": {}, "wlan2": {SSID: "WiFi", Profile: "WiFi-B"}}} {
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
		if decoded.Info.WiFi["wlan2"].Profile != wifi["wlan2"].Profile {
			t.Fatalf("profile = %q, want %q", decoded.Info.WiFi["wlan2"].Profile, wifi["wlan2"].Profile)
		}
		encoded, err = json.Marshal(decoded.Info)
		if err != nil {
			t.Fatal(err)
		}
		var info map[string]any
		if err = json.Unmarshal(encoded, &info); err != nil {
			t.Fatal(err)
		}
		if _, ok := info["wf"]; ok != (len(wifi) > 0) {
			t.Fatalf("wf present = %v for snapshot %v", ok, wifi)
		}
		if wf, ok := info["wf"].(map[string]any); ok {
			if profile := wf["wlan2"].(map[string]any)["p"]; profile != "WiFi-B" {
				t.Fatalf("json profile = %v", profile)
			}
		}
	}
}
