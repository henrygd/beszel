package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/henrygd/beszel/internal/hub/monitors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"gopkg.in/yaml.v3"
)

type monitorsFile struct {
	Monitors []monitorConfig `yaml:"monitors"`
}

type monitorConfig struct {
	Name        string         `yaml:"name"`
	Type        string         `yaml:"type"`
	Target      string         `yaml:"target"`
	Interval    *int           `yaml:"interval,omitempty"`
	Timeout     *int           `yaml:"timeout,omitempty"`
	MaxRetries  *int           `yaml:"max_retries,omitempty"`
	UpsideDown  bool           `yaml:"upside_down,omitempty"`
	Paused      bool           `yaml:"paused,omitempty"`
	Notify      *bool          `yaml:"notify,omitempty"`
	ResendAfter *int           `yaml:"resend_after,omitempty"`
	Users       []string       `yaml:"users"`
	Config      map[string]any `yaml:"config,omitempty"`
}

// SyncMonitors creates/updates monitors from config.yml (monitors: key).
// Unlike SyncSystems it NEVER deletes records absent from the file: monitors
// are UI-first and must survive a YAML sync that does not mention them.
// Matching is done on (name, target).
func SyncMonitors(e *core.ServeEvent) error {
	h := e.App
	configPath := filepath.Join(h.DataDir(), "config.yml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var file monitorsFile
	if err := yaml.Unmarshal(configData, &file); err != nil {
		return fmt.Errorf("failed to parse config.yml monitors: %v", err)
	}
	if len(file.Monitors) == 0 {
		return nil
	}

	userEmailToID := make(map[string]string)
	users, err := h.FindAllRecords("users", dbx.NewExp("id != ''"))
	if err != nil {
		return err
	}
	var firstUser *core.Record
	if len(users) > 0 {
		firstUser = users[0]
		for _, user := range users {
			userEmailToID[user.GetString("email")] = user.Id
		}
	}

	monitorsCollection, err := h.FindCollectionByNameOrId("monitors")
	if err != nil {
		return fmt.Errorf("monitors collection not found (migration missing?): %v", err)
	}

	for _, mc := range file.Monitors {
		if mc.Name == "" || mc.Type == "" || mc.Target == "" {
			log.Printf("Skipping monitor with missing name/type/target")
			continue
		}
		in := monitorInputFromConfig(mc)
		if err := validateMonitorInput(in); err != nil {
			log.Printf("Skipping monitor %q: %v", mc.Name, err)
			continue
		}
		userIDs := resolveMonitorUsers(mc.Users, userEmailToID, firstUser)

		existing, err := h.FindFirstRecordByFilter("monitors",
			"name = {:name} && target = {:target}",
			dbx.Params{"name": mc.Name, "target": strings.TrimSpace(mc.Target)})
		if err == nil {
			existing.Set("type", mc.Type)
			existing.Set("target", strings.TrimSpace(mc.Target))
			existing.Set("interval", in.IntervalSeconds)
			existing.Set("timeout", in.TimeoutSeconds)
			existing.Set("max_retries", in.MaxRetries)
			existing.Set("upside_down", in.UpsideDown)
			existing.Set("paused", mc.Paused)
			existing.Set("notify", in.Notify)
			existing.Set("resend_after", in.ResendAfter)
			existing.Set("users", userIDs)
			if len(mc.Config) > 0 {
				existing.Set("config", mc.Config)
			}
			if err := h.Save(existing); err != nil {
				return err
			}
			continue
		}
		rec := core.NewRecord(monitorsCollection)
		rec.Set("name", mc.Name)
		rec.Set("type", mc.Type)
		rec.Set("target", strings.TrimSpace(mc.Target))
		rec.Set("interval", in.IntervalSeconds)
		rec.Set("timeout", in.TimeoutSeconds)
		rec.Set("max_retries", in.MaxRetries)
		rec.Set("upside_down", in.UpsideDown)
		rec.Set("paused", mc.Paused)
		rec.Set("notify", in.Notify)
		rec.Set("resend_after", in.ResendAfter)
		rec.Set("users", userIDs)
		if len(mc.Config) > 0 {
			rec.Set("config", mc.Config)
		} else {
			rec.Set("config", map[string]any{})
		}
		rec.Set("status", "pending")
		if err := h.Save(rec); err != nil {
			return fmt.Errorf("failed to create monitor %q: %v", mc.Name, err)
		}
	}

	log.Println("Monitors synced with config.yml")
	return nil
}

type validatedMonitorInput struct {
	IntervalSeconds int
	TimeoutSeconds  int
	MaxRetries      int
	UpsideDown      bool
	Notify          bool
	ResendAfter     int
}

// yamlSecretKeys are config keys stripped from generated YAML (mirrors the
// API redaction; secrets are managed out of band).
var yamlSecretKeys = map[string]bool{"password": true, "token": true}

func monitorInputFromConfig(mc monitorConfig) validatedMonitorInput {
	interval := 60
	if mc.Interval != nil {
		interval = *mc.Interval
	}
	timeout := 10
	if mc.Timeout != nil {
		timeout = *mc.Timeout
	}
	maxRetries := 2
	if mc.MaxRetries != nil {
		maxRetries = *mc.MaxRetries
	}
	notify := true
	if mc.Notify != nil {
		notify = *mc.Notify
	}
	resendAfter := 0
	if mc.ResendAfter != nil {
		resendAfter = *mc.ResendAfter
	}
	return validatedMonitorInput{
		IntervalSeconds: interval, TimeoutSeconds: timeout,
		MaxRetries: maxRetries, UpsideDown: mc.UpsideDown,
		Notify: notify, ResendAfter: resendAfter,
	}
}

func validateMonitorInput(in validatedMonitorInput) error {
	m := monitors.Monitor{
		Name: "x", Type: monitors.TypePing, Target: "example.com",
		IntervalSeconds: in.IntervalSeconds, TimeoutSeconds: in.TimeoutSeconds,
		MaxRetries: in.MaxRetries,
	}
	return m.Validate()
}

func resolveMonitorUsers(emails []string, emailToID map[string]string, firstUser *core.Record) []string {
	if len(emails) == 0 {
		if firstUser != nil {
			return []string{firstUser.Id}
		}
		return []string{}
	}
	ids := make([]string, 0, len(emails))
	for _, email := range emails {
		if id, ok := emailToID[email]; ok {
			ids = append(ids, id)
		} else {
			log.Printf("User %s not found", email)
		}
	}
	return ids
}
