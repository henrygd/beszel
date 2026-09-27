// Package wifi collects only currently associated station interfaces. Collection
// failures are empty snapshots, never cached connected state.
package wifi

import (
	"context"
	"math"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/internal/entities/system"
)

// validSSID omits non-UTF-8 SSIDs: 802.11 permits arbitrary octets, but CBOR
// text strings require UTF-8. Metadata must never invalidate the whole response.
func validSSID(ssid string) string {
	if !utf8.ValidString(ssid) {
		return ""
	}
	return ssid
}

// Collect uses a single deadline across interface queries where supported.
// Unsupported platforms and denied association access produce no readings;
// later polls retry.
func Collect() map[string]system.WiFi {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return collect(ctx)
}

// Signals reduces a snapshot to the RSSI values stored in stats history.
// Interfaces without an available reading are omitted.
func Signals(snapshot map[string]system.WiFi) map[string]int8 {
	var signals map[string]int8
	for id, reading := range snapshot {
		if reading.Signal == nil {
			continue
		}
		if signals == nil {
			signals = make(map[string]int8, len(snapshot))
		}
		signals[id] = int8(max(math.Round(*reading.Signal), math.MinInt8))
	}
	return signals
}
