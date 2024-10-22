package hub

import (
	"beszel/internal/entities/system"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pocketbase/pocketbase/models"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Systems []SystemConfig `yaml:"systems"`
}

type SystemConfig struct {
	Name  string   `yaml:"name"`
	Host  string   `yaml:"host"`
	Port  string   `yaml:"port"`
	Users []string `yaml:"users"`
}

// Syncs systems with the config.yml file
func (h *Hub) syncSystemsWithConfig() error {
	configPath := filepath.Join(h.app.DataDir(), "config.yml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var config Config
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		return fmt.Errorf("failed to parse config.yml: %v", err)
	}

	if len(config.Systems) == 0 {
		log.Println("No systems defined in config.yml.")
		return nil
	}

	var firstUser *models.Record

	// Create a map of email to user ID
	userEmailToID := make(map[string]string)
	users, err := h.app.Dao().FindRecordsByFilter("users", "id != ''", "created", -1, 0)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		firstUser = users[0]
		for _, user := range users {
			userEmailToID[user.GetString("email")] = user.Id
		}
	}

	// add default settings for systems if not defined in config
	for i := range config.Systems {
		system := &config.Systems[i]
		if system.Port == "" {
			system.Port = "45876"
		}
		if len(users) > 0 && len(system.Users) == 0 {
			// default to first user if none are defined
			system.Users = []string{firstUser.Id}
		} else {
			// Convert email addresses to user IDs
			userIDs := make([]string, 0, len(system.Users))
			for _, email := range system.Users {
				if id, ok := userEmailToID[email]; ok {
					userIDs = append(userIDs, id)
				} else {
					log.Printf("User %s not found", email)
				}
			}
			system.Users = userIDs
		}
	}

	// Get existing systems
	existingSystems, err := h.app.Dao().FindRecordsByFilter("systems", "id != ''", "", -1, 0)
	if err != nil {
		return err
	}

	// Create a map of existing systems for easy lookup
	existingSystemsMap := make(map[string]*models.Record)
	for _, system := range existingSystems {
		key := system.GetString("host") + ":" + system.GetString("port")
		existingSystemsMap[key] = system
	}

	// Process systems from config
	for _, sysConfig := range config.Systems {
		key := sysConfig.Host + ":" + sysConfig.Port
		if existingSystem, ok := existingSystemsMap[key]; ok {
			// Update existing system
			existingSystem.Set("name", sysConfig.Name)
			existingSystem.Set("users", sysConfig.Users)
			existingSystem.Set("port", sysConfig.Port)
			if err := h.app.Dao().SaveRecord(existingSystem); err != nil {
				return err
			}
			delete(existingSystemsMap, key)
		} else {
			// Create new system
			systemsCollection, err := h.app.Dao().FindCollectionByNameOrId("systems")
			if err != nil {
				return fmt.Errorf("failed to find systems collection: %v", err)
			}
			newSystem := models.NewRecord(systemsCollection)
			newSystem.Set("name", sysConfig.Name)
			newSystem.Set("host", sysConfig.Host)
			newSystem.Set("port", sysConfig.Port)
			newSystem.Set("users", sysConfig.Users)
			newSystem.Set("info", system.Info{})
			newSystem.Set("status", "pending")
			if err := h.app.Dao().SaveRecord(newSystem); err != nil {
				return fmt.Errorf("failed to create new system: %v", err)
			}
		}
	}

	// Delete systems not in config
	for _, system := range existingSystemsMap {
		if err := h.app.Dao().DeleteRecord(system); err != nil {
			return err
		}
	}

	log.Println("Systems synced with config.yml")
	return nil
}
