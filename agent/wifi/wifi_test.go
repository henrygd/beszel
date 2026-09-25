package wifi

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/entities/system"
)

func TestSSIDWireSafety(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"home", "home"}, {"网络 café", "网络 café"}, {"", ""},
		{"raw\xffssid", ""}, {"truncated\xe2\x82", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ssid := validSSID(tc.input)
			if ssid != tc.want {
				t.Fatalf("got %q, want %q", ssid, tc.want)
			}
			signal := -50.0
			payload := map[string]system.WiFi{"wlan0": {SSID: ssid, Signal: &signal}}
			wire, err := cbor.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]system.WiFi
			if err := cbor.Unmarshal(wire, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded["wlan0"].Signal == nil || *decoded["wlan0"].Signal != signal || decoded["wlan0"].SSID != tc.want {
				t.Fatal(decoded)
			}
		})
	}
}

func TestSignals(t *testing.T) {
	strong, weak, rounded := -40.0, -200.0, -52.6
	got := Signals(map[string]system.WiFi{
		"wlan0": {SSID: "home", Signal: &strong},
		"wlan1": {Signal: &weak},
		"wlan2": {Signal: &rounded},
		"wlan3": {SSID: "no rssi"},
	})
	want := map[string]int8{"wlan0": -40, "wlan1": -128, "wlan2": -53}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for id, signal := range want {
		if got[id] != signal {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if Signals(map[string]system.WiFi{"wlan0": {}}) != nil || Signals(nil) != nil {
		t.Fatal("expected nil without available readings")
	}
}
