//go:build testing
// +build testing

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/henrygd/beszel/src/tests"

	"github.com/henrygd/beszel/src/hub/config"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Config struct for testing (copied from config package since it's not exported)
type testConfig struct {
	Systems []testSystemConfig `yaml:"systems"`
}

type testSystemConfig struct {
	Name  string   `yaml:"name"`
	Host  string   `yaml:"host"`
	Port  uint16   `yaml:"port,omitempty"`
	Users []string `yaml:"users"`
	Token string   `yaml:"token,omitempty"`
}

// Helper function to create a test system for config tests
// func createConfigTestSystem(app core.App, name, host string, port uint16, userIDs []string) (*core.Record, error) {
// 	systemCollection, err := app.FindCollectionByNameOrId("systems")
// 	if err != nil {
// 		return nil, err
// 	}

// 	system := core.NewRecord(systemCollection)
// 	system.Set("name", name)
// 	system.Set("host", host)
// 	system.Set("port", port)
// 	system.Set("users", userIDs)
// 	system.Set("status", "pending")

// 	return system, app.Save(system)
// }

// Helper function to create a fingerprint record
func createConfigTestFingerprint(app core.App, systemID, token, fingerprint string) (*core.Record, error) {
	fingerprintCollection, err := app.FindCollectionByNameOrId("fingerprints")
	if err != nil {
		return nil, err
	}

	fp := core.NewRecord(fingerprintCollection)
	fp.Set("system", systemID)
	fp.Set("token", token)
	fp.Set("fingerprint", fingerprint)

	return fp, app.Save(fp)
}

// TestConfigSyncWithTokens tests the config.SyncSystems function with various token scenarios
func TestConfigSyncWithTokens(t *testing.T) {
	testHub, err := tests.NewTestHub()
	require.NoError(t, err)
	defer testHub.Cleanup()

	// Create test user
	user, err := tests.CreateUser(testHub.App, "admin@example.com", "testtesttest")
	require.NoError(t, err)

	testCases := []struct {
		name        string
		setupFunc   func() (string, *core.Record, *core.Record) // Returns: existing token, system record, fingerprint record
		configYAML  string
		expectToken string // Expected token after sync
		description string
	}{
		{
			name: "new system with token in config",
			setupFunc: func() (string, *core.Record, *core.Record) {
				return "", nil, nil // No existing system
			},
			configYAML: `systems:
  - name: "new-server"
    host: "new.example.com"
    port: 45876
    users:
      - "admin@example.com"
    token: "explicit-token-123"`,
			expectToken: "explicit-token-123",
			description: "New system should use token from config",
		},
		{
			name: "existing system without token in config (preserve existing)",
			setupFunc: func() (string, *core.Record, *core.Record) {
				// Create existing system and fingerprint
				system, err := tests.CreateRecord(testHub.App, "systems", map[string]any{
					"name":  "preserve-server",
					"host":  "preserve.example.com",
					"port":  45876,
					"users": []string{user.Id},
				})
				require.NoError(t, err)

				fingerprint, err := createConfigTestFingerprint(testHub.App, system.Id, "preserve-token-999", "preserve-fingerprint")
				require.NoError(t, err)

				return "preserve-token-999", system, fingerprint
			},
			configYAML: `systems:
  - name: "preserve-server"
    host: "preserve.example.com"
    port: 45876
    users:
      - "admin@example.com"`,
			expectToken: "preserve-token-999",
			description: "Existing system should preserve original token when config doesn't specify one",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test data
			_, existingSystem, existingFingerprint := tc.setupFunc()

			// Write config file
			configPath := filepath.Join(testHub.DataDir(), "config.yml")
			err := os.WriteFile(configPath, []byte(tc.configYAML), 0644)
			require.NoError(t, err)

			// Create serve event and sync
			event := &core.ServeEvent{App: testHub.App}
			err = config.SyncSystems(event)
			require.NoError(t, err)

			// Parse the config to get the system name for verification
			var configData testConfig
			err = yaml.Unmarshal([]byte(tc.configYAML), &configData)
			require.NoError(t, err)
			require.Len(t, configData.Systems, 1)
			systemName := configData.Systems[0].Name

			// Find the system after sync
			systems, err := testHub.FindRecordsByFilter("systems", "name = {:name}", "", -1, 0, map[string]any{"name": systemName})
			require.NoError(t, err)
			require.Len(t, systems, 1)
			system := systems[0]

			// Find the fingerprint record
			fingerprints, err := testHub.FindRecordsByFilter("fingerprints", "system = {:system}", "", -1, 0, map[string]any{"system": system.Id})
			require.NoError(t, err)
			require.Len(t, fingerprints, 1)
			fingerprint := fingerprints[0]

			// Verify token
			actualToken := fingerprint.GetString("token")
			if tc.expectToken == "" {
				// For generated tokens, just verify it's not empty and is a valid UUID format
				assert.NotEmpty(t, actualToken, tc.description)
				assert.Len(t, actualToken, 36, "Generated token should be UUID format") // UUID length
			} else {
				assert.Equal(t, tc.expectToken, actualToken, tc.description)
			}

			// For existing systems, verify fingerprint is preserved
			if existingFingerprint != nil {
				actualFingerprint := fingerprint.GetString("fingerprint")
				expectedFingerprint := existingFingerprint.GetString("fingerprint")
				assert.Equal(t, expectedFingerprint, actualFingerprint, "Fingerprint should be preserved")
			}

			// Cleanup for next test
			if existingSystem != nil {
				testHub.Delete(existingSystem)
			}
			if existingFingerprint != nil {
				testHub.Delete(existingFingerprint)
			}
			// Clean up the new records
			testHub.Delete(system)
			testHub.Delete(fingerprint)
		})
	}
}

// TestConfigMigrationScenario tests the specific migration scenario mentioned in the discussion
func TestConfigMigrationScenario(t *testing.T) {
	testHub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer testHub.Cleanup()

	// Create test user
	user, err := tests.CreateUser(testHub.App, "admin@example.com", "testtesttest")
	require.NoError(t, err)

	// Simulate migration scenario: system exists with token from migration
	existingSystem, err := tests.CreateRecord(testHub.App, "systems", map[string]any{
		"name":  "migrated-server",
		"host":  "migrated.example.com",
		"port":  45876,
		"users": []string{user.Id},
	})
	require.NoError(t, err)

	migrationToken := "migration-generated-token-123"
	existingFingerprint, err := createConfigTestFingerprint(testHub.App, existingSystem.Id, migrationToken, "existing-fingerprint-from-agent")
	require.NoError(t, err)

	// User exports config BEFORE this update (so no token field in YAML)
	oldConfigYAML := `systems:
  - name: "migrated-server"
    host: "migrated.example.com"
    port: 45876
    users:
      - "admin@example.com"`

	// Write old config file and import
	configPath := filepath.Join(testHub.DataDir(), "config.yml")
	err = os.WriteFile(configPath, []byte(oldConfigYAML), 0644)
	require.NoError(t, err)

	event := &core.ServeEvent{App: testHub.App}
	err = config.SyncSystems(event)
	require.NoError(t, err)

	// Verify the original token is preserved
	updatedFingerprint, err := testHub.FindRecordById("fingerprints", existingFingerprint.Id)
	require.NoError(t, err)

	actualToken := updatedFingerprint.GetString("token")
	assert.Equal(t, migrationToken, actualToken, "Migration token should be preserved when config doesn't specify a token")

	// Verify fingerprint is also preserved
	actualFingerprint := updatedFingerprint.GetString("fingerprint")
	assert.Equal(t, "existing-fingerprint-from-agent", actualFingerprint, "Existing fingerprint should be preserved")

	// Verify system still exists and is updated correctly
	updatedSystem, err := testHub.FindRecordById("systems", existingSystem.Id)
	require.NoError(t, err)
	assert.Equal(t, "migrated-server", updatedSystem.GetString("name"))
	assert.Equal(t, "migrated.example.com", updatedSystem.GetString("host"))
}
