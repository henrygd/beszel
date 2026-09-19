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
