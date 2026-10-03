package records

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
)

func TestWiFiAverageAvailableSamples(t *testing.T) {
	input := []system.Stats{
		{WiFi: map[string]int8{"wlan0": -40}},
		{},
		{WiFi: map[string]int8{"wlan0": -61, "wlan1": -80}},
	}
	result := AverageSystemStatsSlice(input)
	if len(result.WiFi) != 2 || result.WiFi["wlan0"] != -51 || result.WiFi["wlan1"] != -80 {
		t.Fatalf("%#v", result.WiFi)
	}
	if input[0].WiFi["wlan0"] != -40 {
		t.Fatal("mutated input")
	}
	if len(AverageSystemStatsSlice([]system.Stats{{}, {}}).WiFi) != 0 {
		t.Fatal("invented wifi")
	}
}
