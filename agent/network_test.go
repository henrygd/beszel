//go:build testing

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidNic(t *testing.T) {
	tests := []struct {
		name          string
		nicName       string
		config        *NicConfig
		expectedValid bool
	}{
		{
			name:    "Whitelist - NIC in list",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}},
				isBlacklist: false,
			},
			expectedValid: true,
		},
		{
			name:    "Whitelist - NIC not in list",
			nicName: "wlan0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}},
				isBlacklist: false,
			},
			expectedValid: false,
		},
		{
			name:    "Blacklist - NIC in list",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}},
				isBlacklist: true,
			},
			expectedValid: false,
		},
		{
			name:    "Blacklist - NIC not in list",
			nicName: "wlan0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}},
				isBlacklist: true,
			},
			expectedValid: true,
		},
		{
			name:    "Whitelist with wildcard - matching pattern",
			nicName: "eth1",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:    "Whitelist with wildcard - non-matching pattern",
			nicName: "wlan0",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: false,
		},
		{
			name:    "Blacklist with wildcard - matching pattern",
			nicName: "eth1",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
			expectedValid: false,
		},
		{
			name:    "Blacklist with wildcard - non-matching pattern",
			nicName: "wlan0",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:    "Empty whitelist config - no NICs allowed",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{},
				isBlacklist: false,
			},
			expectedValid: false,
		},
		{
			name:    "Empty blacklist config - all NICs allowed",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{},
				isBlacklist: true,
			},
			expectedValid: true,
		},
		{
			name:    "Multiple patterns - exact match",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}, "wlan*": {}},
				isBlacklist: false,
			},
			expectedValid: true,
		},
		{
			name:    "Multiple patterns - wildcard match",
			nicName: "wlan1",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:    "Multiple patterns - no match",
			nicName: "bond0",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidNic(tt.nicName, tt.config)
			assert.Equal(t, tt.expectedValid, result)
		})
	}
}

func TestNewNicConfig(t *testing.T) {
	tests := []struct {
		name        string
		nicsEnvVal  string
		expectedCfg *NicConfig
	}{
		{
			name:       "Empty string",
			nicsEnvVal: "",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:       "Single NIC whitelist",
			nicsEnvVal: "eth0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:       "Multiple NICs whitelist",
			nicsEnvVal: "eth0,wlan0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan0": {}},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:       "Blacklist mode",
			nicsEnvVal: "-eth0,wlan0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan0": {}},
				isBlacklist:  true,
				hasWildcards: false,
			},
		},
		{
			name:       "With wildcards",
			nicsEnvVal: "eth*,wlan0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}, "wlan0": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
		},
		{
			name:       "Blacklist with wildcards",
			nicsEnvVal: "-eth*,wlan0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}, "wlan0": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
		},
		{
			name:       "With whitespace",
			nicsEnvVal: "eth0, wlan0 , eth1",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan0": {}, "eth1": {}},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:       "Only wildcards",
			nicsEnvVal: "eth*,wlan*",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}, "wlan*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
		},
		{
			name:       "Leading dash only",
			nicsEnvVal: "-",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{},
				isBlacklist:  true,
				hasWildcards: false,
			},
		},
		{
			name:       "Mixed exact and wildcard",
			nicsEnvVal: "eth0,br-*",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "br-*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newNicConfig(tt.nicsEnvVal)
			require.NotNil(t, cfg)
			assert.Equal(t, tt.expectedCfg.isBlacklist, cfg.isBlacklist)
			assert.Equal(t, tt.expectedCfg.hasWildcards, cfg.hasWildcards)
			assert.Equal(t, tt.expectedCfg.nics, cfg.nics)
		})
	}
}
