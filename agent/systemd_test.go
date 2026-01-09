//go:build linux && testing

package agent

import (
	"os"
	"strings"
	"testing"

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

	// Check if either the /run/systemd/system directory exists or PID 1 is systemd
	runSystemdExists := false
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		runSystemdExists = true
	}

	pid1IsSystemd := false
	if data, err := os.ReadFile("/proc/1/comm"); err == nil {
		pid1IsSystemd = strings.TrimSpace(string(data)) == "systemd"
	}

	expected := runSystemdExists || pid1IsSystemd

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing env vars
			os.Unsetenv("BESZEL_AGENT_SERVICE_PATTERNS")
			os.Unsetenv("SERVICE_PATTERNS")

			// Set up environment variables
			if tt.prefixedEnv != "" {
				os.Setenv("BESZEL_AGENT_SERVICE_PATTERNS", tt.prefixedEnv)
			}
			if tt.unprefixedEnv != "" {
				os.Setenv("SERVICE_PATTERNS", tt.unprefixedEnv)
			}

			// Run the function
			result := getServicePatterns()

			// Verify results
			assert.Equal(t, tt.expected, result, "Patterns should match expected values")

			// Cleanup
			if tt.cleanupEnvVars {
				os.Unsetenv("BESZEL_AGENT_SERVICE_PATTERNS")
				os.Unsetenv("SERVICE_PATTERNS")
			}
		})
	}
}
