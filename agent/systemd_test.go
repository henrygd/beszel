//go:build linux && testing

package agent

import (
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
