// Package config provides functions for syncing systems with the config.yml file
package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cast"
	"gopkg.in/yaml.v3"
)

type config struct {
	Systems []systemConfig `yaml:"systems"`
}

type systemConfig struct {
	Name  string `yaml:"name"`
	Host  string `yaml:"host"`
	Port  uint16 `yaml:"port,omitempty"`
	Token string `yaml:"token,omitempty"`
}

// Syncs systems with the config.yml file
func SyncSystems(e *core.ServeEvent) error {
	h := e.App
	configPath := filepath.Join(h.DataDir(), "config.yml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var config config
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		return fmt.Errorf("failed to parse config.yml: %v", err)
	}

	if len(config.Systems) == 0 {
		log.Println("No systems defined in config.yml.")
		return nil
	}

	// add default settings for systems if not defined in config
	for i := range config.Systems {
		system := &config.Systems[i]
		if system.Port == 0 {
			system.Port = 45876
		}
	}

	// Get existing systems
	existingSystems, err := h.FindAllRecords("systems", dbx.NewExp("id != ''"))
	if err != nil {
		return err
	}

	// Create a map of existing systems
	existingSystemsMap := make(map[string]*core.Record)
	for _, system := range existingSystems {
		key := system.GetString("name") + system.GetString("host") + system.GetString("port")
		existingSystemsMap[key] = system
	}

	// Process systems from config
	for _, sysConfig := range config.Systems {
		key := sysConfig.Name + sysConfig.Host + cast.ToString(sysConfig.Port)
		if existingSystem, ok := existingSystemsMap[key]; ok {
			// Update existing system
			existingSystem.Set("name", sysConfig.Name)
			existingSystem.Set("port", sysConfig.Port)
			if err := h.Save(existingSystem); err != nil {
				return err
			}

			// Only update token if one is specified in config, otherwise preserve existing token
			if sysConfig.Token != "" {
				if err := updateFingerprintToken(h, existingSystem.Id, sysConfig.Token); err != nil {
					return err
				}
			}

			delete(existingSystemsMap, key)
		} else {
			// Create new system
			systemsCollection, err := h.FindCollectionByNameOrId("systems")
			if err != nil {
				return fmt.Errorf("failed to find systems collection: %v", err)
			}
			newSystem := core.NewRecord(systemsCollection)
			newSystem.Set("name", sysConfig.Name)
			newSystem.Set("host", sysConfig.Host)
			newSystem.Set("port", sysConfig.Port)
			newSystem.Set("info", system.Info{})
			newSystem.Set("status", "pending")
			if err := h.Save(newSystem); err != nil {
				return fmt.Errorf("failed to create new system: %v", err)
			}

			// For new systems, generate token if not provided
			token := sysConfig.Token
			if token == "" {
				token = uuid.New().String()
			}

			// Create fingerprint record for new system
			if err := createFingerprintRecord(h, newSystem.Id, token); err != nil {
				return err
			}
		}
	}

	// Delete systems not in config (and their fingerprint records will cascade delete)
	for _, system := range existingSystemsMap {
		if err := h.Delete(system); err != nil {
			return err
		}
	}

	log.Println("Systems synced with config.yml")
	return nil
}

// Generates content for the config.yml file as a YAML string
func generateYAML(h core.App) (string, error) {
	// Fetch all systems from the database
	systems, err := h.FindRecordsByFilter("systems", "id != ''", "name", -1, 0)
	if err != nil {
		return "", err
	}

	// Fetch all fingerprint records to get tokens
	type fingerprintData struct {
		ID     string `db:"id"`
		System string `db:"system"`
		Token  string `db:"token"`
	}
	var fingerprints []fingerprintData
	err = h.DB().NewQuery("SELECT id, system, token FROM fingerprints").All(&fingerprints)
	if err != nil {
		return "", err
	}

	// Create a map of system ID to token
	systemTokenMap := make(map[string]string)
	for _, fingerprint := range fingerprints {
		systemTokenMap[fingerprint.System] = fingerprint.Token
	}

	// Populate the Config struct with system data
	config := config{
		Systems: make([]systemConfig, 0, len(systems)),
	}
	for _, system := range systems {
		sysConfig := systemConfig{
			Name:  system.GetString("name"),
			Host:  system.GetString("host"),
			Port:  cast.ToUint16(system.Get("port")),
			Token: systemTokenMap[system.Id],
		}
		config.Systems = append(config.Systems, sysConfig)
	}

	// Marshal the Config struct to YAML
	yamlData, err := yaml.Marshal(&config)
	if err != nil {
		return "", err
	}

	// Add a header to the YAML
	yamlData = append([]byte("# Values for port and token are optional.\n# Defaults are port 45876 and a generated UUID token.\n\n"), yamlData...)

	return string(yamlData), nil
}

// Helper function to update or create fingerprint token for an existing system
func updateFingerprintToken(app core.App, systemID, token string) error {
	// Try to find existing fingerprint record
	fingerprint, err := app.FindFirstRecordByFilter("fingerprints", "system = {:system}", dbx.Params{"system": systemID})
	if err != nil {
		// If no fingerprint record exists, create one
		return createFingerprintRecord(app, systemID, token)
	}

	// Update existing fingerprint record with new token (keep existing fingerprint)
	fingerprint.Set("token", token)
	return app.Save(fingerprint)
}

// Helper function to create a new fingerprint record for a system
func createFingerprintRecord(app core.App, systemID, token string) error {
	fingerprintsCollection, err := app.FindCollectionByNameOrId("fingerprints")
	if err != nil {
		return fmt.Errorf("failed to find fingerprints collection: %v", err)
	}

	newFingerprint := core.NewRecord(fingerprintsCollection)
	newFingerprint.Set("system", systemID)
	newFingerprint.Set("token", token)
	newFingerprint.Set("fingerprint", "") // Empty fingerprint, will be set on first connection

	return app.Save(newFingerprint)
}

// Returns the current config.yml file as a JSON object
func GetYamlConfig(e *core.RequestEvent) error {
	configContent, err := generateYAML(e.App)
	if err != nil {
		return err
	}
	return e.JSON(200, map[string]string{"config": configContent})
}
