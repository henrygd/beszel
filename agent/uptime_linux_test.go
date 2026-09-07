//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetUptimeFromProc(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     uint64
	}{
		{"typical", "12345.67 98765.43\n", 12345},
		{"zero", "0.00 0.00\n", 0},
		{"no trailing newline", "42.99 7.00", 42},
		{"single field", "600.5", 600},
		{"large value", "266030.12 1000000.00\n", 266030},
	}

	prev := uptimeFilePath
	t.Cleanup(func() { uptimeFilePath = prev })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "uptime")
			if err := os.WriteFile(path, []byte(tt.contents), 0o644); err != nil {
				t.Fatal(err)
			}
			uptimeFilePath = path

			got, err := getUptime()
			if err != nil {
				t.Fatalf("getUptime() returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("getUptime() = %d, want %d", got, tt.want)
			}
		})
	}
}

func writeUptime(contents string) func(t *testing.T) string {
	return func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "uptime")
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
}

// Malformed, missing, or out-of-range input must fall back to host.Uptime()
// rather than returning a bogus value, so the agent still reports something sane.
func TestGetUptimeFallsBack(t *testing.T) {
	prev := uptimeFilePath
	t.Cleanup(func() { uptimeFilePath = prev })

	for _, tt := range []struct {
		name    string
		prepare func(t *testing.T) string
	}{
		{"missing file", func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "does-not-exist")
		}},
		{"empty file", func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "uptime")
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}},
		{"unparseable", func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "uptime")
			if err := os.WriteFile(path, []byte("not-a-number 1.0\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}},
		{"NaN", writeUptime("NaN 1.0\n")},
		{"positive infinity", writeUptime("+Inf 1.0\n")},
		{"negative infinity", writeUptime("-Inf 1.0\n")},
		{"negative", writeUptime("-42.5 1.0\n")},
		{"exceeds uint64 range", writeUptime("1e20 1.0\n")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			uptimeFilePath = tt.prepare(t)

			got, err := getUptime()
			if err != nil {
				t.Fatalf("getUptime() returned error: %v", err)
			}
			if got == 0 {
				t.Error("getUptime() = 0, expected fallback to host.Uptime()")
			}
		})
	}
}
