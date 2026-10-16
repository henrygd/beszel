package systems

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type smartFetchState struct {
	LastAttempt int64
	Successful  bool
}

// FetchAndSaveSmartDevices fetches SMART data from the agent and saves it to the database
func (sys *System) FetchAndSaveSmartDevices() error {
	response, err := sys.FetchSmartDataFromAgent()
	if err != nil {
		sys.recordSmartFetchResult(err, 0)
		return err
	}
	err = sys.saveSmartDevices(response.Data, response.Complete)
	sys.recordSmartFetchResult(err, len(response.Data))
	return err
}

// recordSmartFetchResult stores a cooldown entry for the SMART interval and marks
// whether the last fetch produced any devices, so failed setup can retry on reconnect.
func (sys *System) recordSmartFetchResult(err error, deviceCount int) {
	if sys.manager == nil {
		return
	}
	interval := sys.smartFetchInterval()
	success := err == nil && deviceCount > 0
	if sys.manager.hub != nil {
		sys.manager.hub.Logger().Info("SMART fetch result", "system", sys.Id, "success", success, "devices", deviceCount, "interval", interval.String(), "err", err)
	}
	sys.manager.smartFetchMap.Set(sys.Id, smartFetchState{LastAttempt: time.Now().UnixMilli(), Successful: success}, interval+time.Minute)
}

// shouldFetchSmart returns true when there is no active SMART cooldown entry for this system.
func (sys *System) shouldFetchSmart() bool {
	if sys.manager == nil {
		return true
	}
	state, ok := sys.manager.smartFetchMap.GetOk(sys.Id)
	if !ok {
		return true
	}
	return !time.UnixMilli(state.LastAttempt).Add(sys.smartFetchInterval()).After(time.Now())
}

// smartFetchInterval returns the agent-provided SMART interval or the default when unset.
func (sys *System) smartFetchInterval() time.Duration {
	if sys.smartInterval > 0 {
		return sys.smartInterval
	}
	return time.Hour
}

// saveSmartDevices saves SMART device data and, after a complete refresh,
// removes rows for devices that are no longer reported.
func (sys *System) saveSmartDevices(smartData map[string]smart.SmartData, complete bool) error {
	if len(smartData) == 0 {
		return nil
	}

	hub := sys.manager.hub
	collection, err := hub.FindCachedCollectionByNameOrId("smart_devices")
	if err != nil {
		return err
	}

	currentIDs := make(map[string]struct{}, len(smartData))
	for deviceKey := range smartData {
		currentIDs[MakeStableHashId(sys.Id, deviceKey)] = struct{}{}
	}

	err = hub.RunInTransaction(func(txApp core.App) error {
		if complete {
			existing, err := txApp.FindRecordsByFilter(
				collection,
				"system = {:system}",
				"",
				0,
				0,
				dbx.Params{"system": sys.Id},
			)
			if err != nil {
				return err
			}
			for _, record := range existing {
				if _, ok := currentIDs[record.Id]; ok {
					continue
				}
				if err := txApp.Delete(record); err != nil {
					return err
				}
			}
		}

		for deviceKey, device := range smartData {
			if err := sys.upsertSmartDeviceRecord(txApp, collection, deviceKey, device); err != nil {
				return err
			}
		}

		return nil
	})
	return err
}

func (sys *System) upsertSmartDeviceRecord(app core.App, collection *core.Collection, deviceKey string, device smart.SmartData) error {
	recordID := MakeStableHashId(sys.Id, deviceKey)

	record, err := app.FindRecordById(collection, recordID)
	existingRecord := err == nil
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		record = core.NewRecord(collection)
		record.Set("id", recordID)
	}

	name := device.DiskName
	if name == "" {
		name = deviceKey
	}

	now := time.Now().UTC()
	var trendState smart.SmartTrendState
	decodeRecordJSON(record, "smart_history", &trendState)
	// A pre-migration row contains only its latest SMART snapshot. Preserve that
	// one available baseline before replacing the record with the current sample.
	if existingRecord && len(trendState.Samples) == 0 {
		var previousAttributes []*smart.SmartAttribute
		decodeRecordJSON(record, "attributes", &previousAttributes)
		if previousUpdated := record.GetDateTime("updated").Time().UTC(); !previousUpdated.IsZero() && previousUpdated.Before(now) {
			trendState, _ = smart.EvaluateSmartHealth(previousUpdated, record.GetString("serial"), record.GetString("state"), previousAttributes, trendState)
		}
	}
	trendState, health := smart.EvaluateSmartHealth(now, device.SerialNumber, device.SmartStatus, device.Attributes, trendState)

	powerOnHours, powerCycles := extractPowerMetrics(device.Attributes)
	record.Set("system", sys.Id)
	record.Set("name", name)
	record.Set("model", device.ModelName)
	record.Set("state", health.Status)
	record.Set("capacity", device.Capacity)
	record.Set("temp", device.Temperature)
	record.Set("firmware", device.FirmwareVersion)
	record.Set("serial", device.SerialNumber)
	record.Set("type", device.DiskType)
	record.Set("hours", powerOnHours)
	record.Set("cycles", powerCycles)
	record.Set("attributes", device.Attributes)
	record.Set("smart_history", trendState)
	record.Set("smart_health", health)

	return app.SaveNoValidate(record)
}

func decodeRecordJSON(record *core.Record, field string, dest any) {
	raw := record.Get(field)
	if raw == nil {
		return
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, dest)
}

// extractPowerMetrics extracts power on hours and power cycles from SMART attributes
func extractPowerMetrics(attributes []*smart.SmartAttribute) (powerOnHours, powerCycles uint64) {
	for _, attr := range attributes {
		nameLower := strings.ToLower(attr.Name)
		if powerOnHours == 0 && (strings.Contains(nameLower, "poweronhours") || strings.Contains(nameLower, "power_on_hours")) {
			powerOnHours = attr.RawValue
		}
		if powerCycles == 0 && ((strings.Contains(nameLower, "power") && strings.Contains(nameLower, "cycle")) || strings.Contains(nameLower, "startstopcycles")) {
			powerCycles = attr.RawValue
		}
		if powerOnHours > 0 && powerCycles > 0 {
			break
		}
	}
	return
}
