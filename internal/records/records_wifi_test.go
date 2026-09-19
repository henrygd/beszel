package records

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
)

func TestWiFiAverageAvailableSamples(t *testing.T) {
	a, b, c := -40.0, -60.0, -80.0
	input := []system.Stats{
		{WiFi: map[string]system.WiFi{"wlan0": {SSID: "old", Signal: &a}}},
		{},
		{WiFi: map[string]system.WiFi{"wlan0": {SSID: "new", Signal: &b}, "wlan1": {Signal: &c}, "unknown": {}}},
	}
	result := AverageSystemStatsSlice(input)
	if len(result.WiFi) != 3 || *result.WiFi["wlan0"].Signal != -50 || *result.WiFi["wlan1"].Signal != -80 || result.WiFi["unknown"].Signal != nil || result.WiFi["wlan0"].SSID != "new" {
		t.Fatalf("%#v", result.WiFi)
	}
	if *input[0].WiFi["wlan0"].Signal != -40 {
		t.Fatal("mutated input")
	}
	if len(AverageSystemStatsSlice([]system.Stats{{}, {}}).WiFi) != 0 {
		t.Fatal("invented wifi")
	}
}
