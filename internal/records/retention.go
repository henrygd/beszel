package records

import (
	"log/slog"
	"time"

	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/pocketbase/pocketbase/core"
)

// retentionOptions maps retention string values to durations
// "never" is represented as 0 which means no deletion
var retentionDurations = map[string]time.Duration{
	"30d":   30 * 24 * time.Hour,
	"60d":   60 * 24 * time.Hour,
	"90d":   90 * 24 * time.Hour,
	"180d":  180 * 24 * time.Hour,
	"365d":  365 * 24 * time.Hour,
	"730d":  730 * 24 * time.Hour,
	"1095d": 1095 * 24 * time.Hour,
	"1825d": 1825 * 24 * time.Hour,
	"never": 0,
}

// ValidRetentions is the canonical list of allowed retention values (single source of truth)
var ValidRetentions = []string{"30d", "60d", "90d", "180d", "365d", "730d", "1095d", "1825d", "never"}

// GetRetentionDuration returns the retention duration for 480m records
// Priority: env var BESZEL_HUB_RETENTION > hub_settings DB > default 30d
func GetRetentionDuration(app core.App) time.Duration {
	// env var override - check BESZEL_HUB_RETENTION or RETENTION
	if val, ok := utils.GetEnv("RETENTION"); ok && val != "" {
		if d, exists := retentionDurations[val]; exists {
			// "never" from env is handled as 0
			return d
		}
		slog.Warn("Invalid RETENTION env value, falling back to DB", "value", val)
	}

	// DB value
	retention := getRetentionFromDB(app)
	if d, ok := retentionDurations[retention]; ok {
		return d
	}
	// fallback
	return 30 * 24 * time.Hour
}

// GetRetentionString returns the raw retention string (e.g. "30d", "365d", "never")
func GetRetentionString(app core.App) string {
	if val, ok := utils.GetEnv("RETENTION"); ok && val != "" {
		if _, exists := retentionDurations[val]; exists {
			return val
		}
	}
	retention := getRetentionFromDB(app)
	if _, ok := retentionDurations[retention]; ok {
		return retention
	}
	return "30d"
}

func getRetentionFromDB(app core.App) string {
	// hub_settings may not exist during migrations or tests
	record, err := app.FindRecordById("hub_settings", "hubsettings0001")
	if err == nil {
		val := record.GetString("retention")
		if val != "" {
			return val
		}
		return "30d"
	}
	// fallback: try any record deterministically ordered by created
	records, err2 := app.FindRecordsByFilter("hub_settings", "", "created", 1, 0, nil)
	if err2 != nil || len(records) == 0 {
		return "30d"
	}
	val := records[0].GetString("retention")
	if val == "" {
		return "30d"
	}
	return val
}

// GetDbRetention returns the raw DB value without env override (for /api/beszel/retention)
func GetDbRetention(app core.App) string {
	return getRetentionFromDB(app)
}

// IsEnvOverride reports whether RETENTION env var is active
func IsEnvOverride() bool {
	val, ok := utils.GetEnv("RETENTION")
	if !ok || val == "" {
		return false
	}
	_, exists := retentionDurations[val]
	return exists
}

// EnsureHubSettingsExists creates default hub_settings record if missing
// Called on hub initialization
func EnsureHubSettingsExists(app core.App) error {
	// check if collection exists
	_, err := app.FindCollectionByNameOrId("hub_settings")
	if err != nil {
		// collection not yet migrated (e.g. tests with no_ui), skip
		return nil
	}
	count, err := app.CountRecords("hub_settings")
	if err != nil {
		return err
	}
	if count > 1 {
		// enforce singleton: keep oldest (created ASC), delete extras (#1)
		records, err := app.FindRecordsByFilter("hub_settings", "", "created", 0, 0, nil)
		if err == nil && len(records) > 1 {
			for _, r := range records[1:] {
				_ = app.Delete(r)
			}
		}
		return nil
	}
	if count > 0 {
		return nil
	}
	collection, err := app.FindCollectionByNameOrId("hub_settings")
	if err != nil {
		return err
	}
	// check env for default
	defaultRetention := "30d"
	if val, ok := utils.GetEnv("RETENTION"); ok && val != "" {
		if _, exists := retentionDurations[val]; exists {
			defaultRetention = val
		}
	}
	record := core.NewRecord(collection)
	record.Set("id", "hubsettings0001")
	record.Set("retention", defaultRetention)
	return app.Save(record)
}
