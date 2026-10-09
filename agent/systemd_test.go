//go:build linux && testing

package agent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/entities/systemd"
	"github.com/stretchr/testify/assert"
)

func TestUnescapeServiceName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"nginx.service", "nginx.service"},                                     // No escaping needed
		{"test\\x2dwith\\x2ddashes.service", "test-with-dashes.service"},       // \x2d is dash
		{"service\\x20with\\x20spaces.service", "service with spaces.service"}, // \x20 is space
		{"mixed\\x2dand\\x2dnormal", "mixed-and-normal"},                       // Mixed escaped and normal
		{"no-escape-here", "no-escape-here"},                                   // No escape sequences
		{"", ""},                                                               // Empty string
		{"\\x2d\\x2d", "--"},                                                   // Multiple escapes
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := unescapeServiceName(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestLimitedBuffer(t *testing.T) {
	buffer := limitedBuffer{limit: 5}

	n, err := buffer.Write([]byte("abcdef"))
	assert.Equal(t, 5, n)
	assert.ErrorIs(t, err, errSystemdLogLimitReached)
	assert.Equal(t, "abcde", buffer.String())

	n, err = buffer.Write([]byte("g"))
	assert.Zero(t, n)
	assert.True(t, errors.Is(err, errSystemdLogLimitReached))
}

func TestLimitedBufferCapsExecOutput(t *testing.T) {
	buffer := limitedBuffer{limit: 5}
	cmd := exec.Command("sh", "-c", "printf 'abcdef'")
	cmd.Stdout = &buffer

	err := cmd.Run()
	assert.ErrorIs(t, err, errSystemdLogLimitReached)
	assert.Equal(t, "abcde", buffer.String())
}

func TestServiceUnitName(t *testing.T) {
	tests := map[string]string{
		"nginx":         "nginx.service",
		"nginx.service": "nginx.service",
		"backup.timer":  "backup.timer",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, want, serviceUnitName(input))
		})
	}
}

func TestCanReadSystemJournal(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   bool
	}{
		{"readable", "#!/bin/sh\nprintf 'system log\\n'\n", true},
		{"empty", "#!/bin/sh\nexit 0\n", true},
		{"denied", "#!/bin/sh\nexit 1\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(test.script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			assert.Equal(t, test.want, canReadSystemJournal())
		})
	}
}

func TestGetServiceLogsOnlyMonitoredUnits(t *testing.T) {
	// Fake journalctl prints the unit it was asked for
	dir := t.TempDir()
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = --unit ] && printf '%s' \"$2\"; shift; done\n"
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	sm := &systemdManager{logsEnabled: true, serviceStatsMap: map[string]*systemd.Service{
		"nginx.service":       {Name: "nginx"},
		"backup.timer":        {Name: "backup.timer"},
		"foo\\x2dbar.service": {Name: "foo-bar"},
		"getty@tty1.service":  {Name: "getty@tty1"},
	}}

	tests := []struct {
		name string
		want string
	}{
		{"nginx", "nginx.service"},
		{"nginx.service", "nginx.service"},
		{"backup.timer", "backup.timer"},
		{"foo-bar", "foo\\x2dbar.service"},
		{"getty@tty1", "getty@tty1.service"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs, err := sm.getServiceLogs(test.name)
			assert.NoError(t, err)
			assert.Equal(t, test.want, logs)
		})
	}

	for _, name := range []string{"sshd", "*", "*.service", "nginx*"} {
		t.Run("rejects "+name, func(t *testing.T) {
			logs, err := sm.getServiceLogs(name)
			assert.Error(t, err)
			assert.Empty(t, logs)
		})
	}

	t.Run("disabled", func(t *testing.T) {
		sm.logsEnabled = false
		logs, err := sm.getServiceLogs("nginx")
		assert.Error(t, err)
		assert.Empty(t, logs)
	})
}

func TestSystemdLogsEnabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	assert.True(t, systemdLogsEnabled())

	t.Setenv("SKIP_SYSTEMD_LOGS", "true")
	assert.False(t, systemdLogsEnabled())
}

func TestUnescapeServiceNameInvalid(t *testing.T) {
	// Test invalid escape sequences - should return original string
	invalidInputs := []string{
		"invalid\\x",   // Incomplete escape
		"invalid\\xZZ", // Invalid hex
		"invalid\\x2",  // Incomplete hex
		"invalid\\xyz", // Not a valid escape
	}

	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			result := unescapeServiceName(input)
			assert.Equal(t, input, result, "Invalid escape sequences should return original string")
		})
	}
}

func TestIsSystemdAvailable(t *testing.T) {
	// Note: This test's result will vary based on the actual system running the tests
	// On systems with systemd, it should return true
	// On systems without systemd, it should return false
	result := isSystemdAvailable()

	// Check if the systemd directory or a dbus socket exists, or PID 1 is systemd
	pathExists := false
	for _, path := range []string{"/run/systemd/system", "/run/dbus/system_bus_socket", "/var/run/dbus/system_bus_socket"} {
		if _, err := os.Stat(path); err == nil {
			pathExists = true
			break
		}
	}

	pid1IsSystemd := false
	if data, err := os.ReadFile("/proc/1/comm"); err == nil {
		pid1IsSystemd = strings.TrimSpace(string(data)) == "systemd"
	}

	expected := pathExists || pid1IsSystemd

	assert.Equal(t, expected, result, "isSystemdAvailable should correctly detect systemd presence")

	// Log the result for informational purposes
	if result {
		t.Log("Systemd is available on this system")
	} else {
		t.Log("Systemd is not available on this system")
	}
}

func TestGetServicePatterns(t *testing.T) {
	tests := []struct {
		name           string
		prefixedEnv    string
		unprefixedEnv  string
		expected       []string
		cleanupEnvVars bool
	}{
		{
			name:           "default when no env var set",
			prefixedEnv:    "",
			unprefixedEnv:  "",
			expected:       []string{"*.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "single pattern with prefixed env",
			prefixedEnv:    "nginx",
			unprefixedEnv:  "",
			expected:       []string{"nginx.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "single pattern with unprefixed env",
			prefixedEnv:    "",
			unprefixedEnv:  "nginx",
			expected:       []string{"nginx.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "prefixed env takes precedence",
			prefixedEnv:    "nginx",
			unprefixedEnv:  "apache",
			expected:       []string{"nginx.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "multiple patterns",
			prefixedEnv:    "nginx,apache,postgresql",
			unprefixedEnv:  "",
			expected:       []string{"nginx.service", "apache.service", "postgresql.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "patterns with .service suffix",
			prefixedEnv:    "nginx.service,apache.service",
			unprefixedEnv:  "",
			expected:       []string{"nginx.service", "apache.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "mixed patterns with and without suffix",
			prefixedEnv:    "nginx.service,apache,postgresql.service",
			unprefixedEnv:  "",
			expected:       []string{"nginx.service", "apache.service", "postgresql.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "patterns with whitespace",
			prefixedEnv:    " nginx , apache , postgresql ",
			unprefixedEnv:  "",
			expected:       []string{"nginx.service", "apache.service", "postgresql.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "empty patterns are skipped",
			prefixedEnv:    "nginx,,apache,  ,postgresql",
			unprefixedEnv:  "",
			expected:       []string{"nginx.service", "apache.service", "postgresql.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "wildcard pattern",
			prefixedEnv:    "*nginx*,*apache*",
			unprefixedEnv:  "",
			expected:       []string{"*nginx*.service", "*apache*.service"},
			cleanupEnvVars: true,
		},
		{
			name:           "opt into timer monitoring",
			prefixedEnv:    "nginx.service,docker,apache.timer",
			unprefixedEnv:  "",
			expected:       []string{"nginx.service", "docker.service", "apache.timer"},
			cleanupEnvVars: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			if tt.prefixedEnv != "" {
				t.Setenv("BESZEL_AGENT_SERVICE_PATTERNS", tt.prefixedEnv)
			}
			if tt.unprefixedEnv != "" {
				t.Setenv("SERVICE_PATTERNS", tt.unprefixedEnv)
			}

			// Run the function
			result := getServicePatterns()

			// Verify results
			assert.Equal(t, tt.expected, result, "Patterns should match expected values")
		})
	}
}
