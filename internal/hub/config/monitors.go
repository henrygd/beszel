package config

import (
	"database/sql"
	"errors"
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
		name := strings.TrimSpace(mc.Name)
		typ := strings.ToLower(strings.TrimSpace(mc.Type))
		if name == "" || typ == "" || strings.TrimSpace(mc.Target) == "" {
			log.Printf("Skipping monitor with missing name/type/target")
			continue
		}
		if !validMonitorType(typ) {
			log.Printf("Skipping monitor %q: unknown type %q", name, mc.Type)
			continue
		}
		in := monitorInputFromConfig(mc)
		if err := validateMonitorInput(in); err != nil {
			log.Printf("Skipping monitor %q: %v", name, err)
			continue
		}
		userIDs := resolveMonitorUsers(mc.Users, userEmailToID, firstUser)
		if len(userIDs) == 0 {
			log.Printf("Skipping monitor %q: no known users", name)
			continue
		}

		existing, err := h.FindFirstRecordByFilter("monitors",
			"name = {:name} && target = {:target}",
			dbx.Params{"name": name, "target": strings.TrimSpace(mc.Target)})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to look up monitor %q: %v", name, err)
		}
		if err == nil {
			applyMonitorConfig(existing, name, typ, strings.TrimSpace(mc.Target), in, mc, userIDs, true)
			if err := h.Save(existing); err != nil {
				log.Printf("Skipping monitor %q: %v", name, err)
				continue
			}
			continue
		}
		rec := core.NewRecord(monitorsCollection)
		applyMonitorConfig(rec, name, typ, strings.TrimSpace(mc.Target), in, mc, userIDs, false)
		rec.Set("status", "pending")
		if err := h.Save(rec); err != nil {
			log.Printf("Skipping monitor %q: %v", name, err)
			continue
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
	if err := m.Validate(); err != nil {
		return err
	}
	if in.ResendAfter < 0 || in.ResendAfter > 1440 {
		return fmt.Errorf("resend_after must be 0..1440 minutes")
	}
	return nil
}

func validMonitorType(t string) bool {
	switch monitors.MonitorType(t) {
	case monitors.TypeHTTP, monitors.TypeKeyword, monitors.TypePing, monitors.TypeDNS, monitors.TypeTLS:
		return true
	}
	return false
}

// applyMonitorConfig writes validated YAML values onto a record. On update,
// stored secrets absent from the YAML are preserved; headers are never
// written from YAML exports (see generateYAML).
func applyMonitorConfig(rec *core.Record, name, typ, target string, in validatedMonitorInput, mc monitorConfig, userIDs []string, isUpdate bool) {
	rec.Set("name", name)
	rec.Set("type", typ)
	rec.Set("target", target)
	rec.Set("interval", in.IntervalSeconds)
	rec.Set("timeout", in.TimeoutSeconds)
	rec.Set("max_retries", in.MaxRetries)
	rec.Set("upside_down", in.UpsideDown)
	rec.Set("paused", mc.Paused)
	rec.Set("notify", in.Notify)
	rec.Set("resend_after", in.ResendAfter)
	rec.Set("users", userIDs)
	if len(mc.Config) == 0 {
		if !isUpdate {
			rec.Set("config", map[string]any{})
		}
		return
	}
	merged := make(map[string]any, len(mc.Config))
	stored := map[string]any{}
	if isUpdate {
		_ = rec.UnmarshalJSONField("config", &stored)
		for k, v := range stored {
			merged[k] = v
		}
	}
	for k, v := range mc.Config {
		merged[k] = v
	}
	// Never wipe stored secrets via YAML: a secret absent from the YAML
	// keeps its stored value (the merge above already preserves it; the
	// explicit pass documents the guarantee).
	if isUpdate {
		for k := range yamlSecretKeys {
			if _, ok := mc.Config[k]; !ok {
				delete(merged, k)
				if sv, ok := stored[k]; ok {
					merged[k] = sv
				}
			}
		}
	}
	rec.Set("config", merged)
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
