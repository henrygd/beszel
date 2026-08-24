package systems

import (
	"database/sql"
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
		currentIDs[makeStableHashId(sys.Id, deviceKey)] = struct{}{}
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
	recordID := makeStableHashId(sys.Id, deviceKey)

	record, err := app.FindRecordById(collection, recordID)
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

	powerOnHours, powerCycles := extractPowerMetrics(device.Attributes)
	record.Set("system", sys.Id)
	record.Set("name", name)
	record.Set("model", device.ModelName)
	record.Set("state", device.SmartStatus)
	record.Set("capacity", device.Capacity)
	record.Set("temp", device.Temperature)
	record.Set("firmware", device.FirmwareVersion)
	record.Set("serial", device.SerialNumber)
	record.Set("type", device.DiskType)
	record.Set("hours", powerOnHours)
	record.Set("cycles", powerCycles)
	record.Set("attributes", device.Attributes)

	return app.SaveNoValidate(record)
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
