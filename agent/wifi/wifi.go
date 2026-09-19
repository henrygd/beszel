// Package wifi collects only currently associated station interfaces. Collection
// failures are empty snapshots, never cached connected state.
package wifi

import (
	"context"
	"os"
	"os/exec"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/internal/entities/system"
)

type commandRunner func(context.Context, string, ...string) ([]byte, error)

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd.Output()
}

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
