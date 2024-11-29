package hub

import (
	"beszel/internal/entities/system"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cast"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Systems []SystemConfig `yaml:"systems"`
}

type SystemConfig struct {
	Name  string   `yaml:"name"`
	Host  string   `yaml:"host"`
	Port  uint16   `yaml:"port"`
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

	var firstUser *core.Record

	// Create a map of email to user ID
	userEmailToID := make(map[string]string)
	users, err := h.app.FindAllRecords("users", dbx.NewExp("id != ''"))
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
		if system.Port == 0 {
			system.Port = 45876
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
	existingSystems, err := h.app.FindAllRecords("systems", dbx.NewExp("id != ''"))
	if err != nil {
		return err
	}

	// Create a map of existing systems for easy lookup
	existingSystemsMap := make(map[string]*core.Record)
	for _, system := range existingSystems {
		key := system.GetString("host") + ":" + system.GetString("port")
		existingSystemsMap[key] = system
	}

	// Process systems from config
	for _, sysConfig := range config.Systems {
		key := sysConfig.Host + ":" + strconv.Itoa(int(sysConfig.Port))
		if existingSystem, ok := existingSystemsMap[key]; ok {
			// Update existing system
			existingSystem.Set("name", sysConfig.Name)
			existingSystem.Set("users", sysConfig.Users)
			existingSystem.Set("port", sysConfig.Port)
			if err := h.app.Save(existingSystem); err != nil {
				return err
			}
			delete(existingSystemsMap, key)
		} else {
			// Create new system
			systemsCollection, err := h.app.FindCollectionByNameOrId("systems")
			if err != nil {
				return fmt.Errorf("failed to find systems collection: %v", err)
			}
			newSystem := core.NewRecord(systemsCollection)
			newSystem.Set("name", sysConfig.Name)
			newSystem.Set("host", sysConfig.Host)
			newSystem.Set("port", sysConfig.Port)
			newSystem.Set("users", sysConfig.Users)
			newSystem.Set("info", system.Info{})
			newSystem.Set("status", "pending")
			if err := h.app.Save(newSystem); err != nil {
				return fmt.Errorf("failed to create new system: %v", err)
			}
		}
	}

	// Delete systems not in config
	for _, system := range existingSystemsMap {
		if err := h.app.Delete(system); err != nil {
			return err
		}
	}

	log.Println("Systems synced with config.yml")
	return nil
}

// Generates content for the config.yml file as a YAML string
func (h *Hub) generateConfigYAML() (string, error) {
	// Fetch all systems from the database
	systems, err := h.app.FindRecordsByFilter("systems", "id != ''", "name", -1, 0)
	if err != nil {
		return "", err
	}

	// Create a Config struct to hold the data
	config := Config{
		Systems: make([]SystemConfig, 0, len(systems)),
	}

	// Fetch all users at once
	allUserIDs := make([]string, 0)
	for _, system := range systems {
		allUserIDs = append(allUserIDs, system.GetStringSlice("users")...)
	}
	userEmailMap, err := h.getUserEmailMap(allUserIDs)
	if err != nil {
		return "", err
	}

	// Populate the Config struct with system data
	for _, system := range systems {
		userIDs := system.GetStringSlice("users")
		userEmails := make([]string, 0, len(userIDs))
		for _, userID := range userIDs {
			if email, ok := userEmailMap[userID]; ok {
				userEmails = append(userEmails, email)
			}
		}

		sysConfig := SystemConfig{
			Name:  system.GetString("name"),
			Host:  system.GetString("host"),
			Port:  cast.ToUint16(system.Get("port")),
			Users: userEmails,
		}
		config.Systems = append(config.Systems, sysConfig)
	}

	// Marshal the Config struct to YAML
	yamlData, err := yaml.Marshal(&config)
	if err != nil {
		return "", err
	}

	// Add a header to the YAML
	yamlData = append([]byte("# Values for port and users are optional.\n# Defaults are port 45876 and the first created user.\n\n"), yamlData...)

	return string(yamlData), nil
}

// New helper function to get a map of user IDs to emails
func (h *Hub) getUserEmailMap(userIDs []string) (map[string]string, error) {
	users, err := h.app.FindRecordsByIds("users", userIDs)
	if err != nil {
		return nil, err
	}

	userEmailMap := make(map[string]string, len(users))
	for _, user := range users {
		userEmailMap[user.Id] = user.GetString("email")
	}

	return userEmailMap, nil
}

// Returns the current config.yml file as a JSON object
func (h *Hub) getYamlConfig(e *core.RequestEvent) error {
	info, _ := e.RequestInfo()
	if info.Auth == nil || info.Auth.GetString("role") != "admin" {
		return apis.NewForbiddenError("Forbidden", nil)
	}
	configContent, err := h.generateConfigYAML()
	if err != nil {
		return err
	}
	return e.JSON(200, map[string]string{"config": configContent})
}
