//go:build windows

package wifi

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestProfileName(t *testing.T) {
	for _, tc := range []struct{ profile, ssid, want string }{
		{"WiFi-B", "WiFi", "WiFi-B"},
		{"WiFi", "WiFi", ""},
		{"", "WiFi", ""},
		{"Réseau 网络", "", "Réseau 网络"},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			var profile [256]uint16
			copy(profile[:], windows.StringToUTF16(tc.profile))
			if got := profileName(profile[:], tc.ssid); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
