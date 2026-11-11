package systems

import (
	"database/sql"
	"errors"
	"time"

	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/entities/nut"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type nutFetchState struct {
	LastAttempt int64
	Successful  bool
}

// supportsNutData reports whether the connected agent is new enough to serve NUT data.
func (sys *System) supportsNutData() bool {
	return sys.getAgentVersion().GTE(beszel.MinVersionNutData)
}

// FetchAndSaveNutDevices fetches NUT (UPS/PDU) data from the agent and saves it to the database.
func (sys *System) FetchAndSaveNutDevices() error {
	response, err := sys.FetchNutDataFromAgent()
	if err != nil {
		sys.recordNutFetchResult(err, 0)
		return err
	}
	sampled := response.Sampled
	if sampled == nil {
		// Older agents do not report which devices were collected this run.
		sampled = make([]string, 0, len(response.Data))
		for deviceKey := range response.Data {
			sampled = append(sampled, deviceKey)
		}
	}
	err = sys.saveNutDevices(response.Data, response.Complete, sampled)
	sys.recordNutFetchResult(err, len(sampled))
	return err
}

// recordNutFetchResult stores a cooldown entry for the NUT interval and marks
// whether the last fetch produced any devices, so failed setup can retry on reconnect.
func (sys *System) recordNutFetchResult(err error, deviceCount int) {
	if sys.manager == nil {
		return
	}
	interval := sys.nutFetchInterval()
	success := err == nil && deviceCount > 0
	if sys.manager.hub != nil {
		sys.manager.hub.Logger().Info("NUT fetch result", "system", sys.Id, "success", success, "devices", deviceCount, "interval", interval.String(), "err", err)
	}
	sys.manager.nutFetchMap.Set(sys.Id, nutFetchState{LastAttempt: time.Now().UnixMilli(), Successful: success}, interval+time.Minute)
}

// shouldFetchNut returns true when there is no active NUT cooldown entry for this system.
func (sys *System) shouldFetchNut() bool {
	if sys.manager == nil {
		return true
	}
	state, ok := sys.manager.nutFetchMap.GetOk(sys.Id)
	if !ok {
		return true
	}
	return !time.UnixMilli(state.LastAttempt).Add(sys.nutFetchInterval()).After(time.Now())
}

// nutFetchInterval returns the agent-provided NUT interval or the default when unset.
func (sys *System) nutFetchInterval() time.Duration {
	if sys.nutInterval > 0 {
		return sys.nutInterval
	}
	return time.Hour
}

// saveNutDevices saves NUT device data and, after a complete refresh,
// removes rows for devices that are no longer reported. Devices in sampled
// were collected during this refresh; devices only present in the agent's
// persistent cache are not rewritten or added to history, so a failed
// collection is not presented as a new measurement.
func (sys *System) saveNutDevices(nutData map[string]nut.NutData, complete bool, sampled []string) error {
	hub := sys.manager.hub
	collection, err := hub.FindCachedCollectionByNameOrId("nut_devices")
	if err != nil {
		return err
	}

	sampledSet := make(map[string]bool, len(sampled))
	for _, name := range sampled {
		sampledSet[name] = true
	}
	if sampled == nil {
		// Callers that do not report which devices were collected (older agents)
		// are treated as fully fresh data.
		for deviceKey := range nutData {
			sampledSet[deviceKey] = true
		}
	}

	currentIDs := make(map[string]struct{}, len(nutData))
	for deviceKey := range nutData {
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

		statsCollection, err := txApp.FindCachedCollectionByNameOrId("nut_stats")
		if err != nil {
			return err
		}
		for deviceKey, device := range nutData {
			fresh := sampledSet[deviceKey]
			if err := sys.upsertNutDeviceRecord(txApp, collection, deviceKey, device, fresh); err != nil {
				return err
			}
			if !fresh {
				continue
			}
			if err := sys.createNutStatsRecord(txApp, statsCollection, deviceKey, device); err != nil {
				return err
			}
		}

		return nil
	})
	return err
}

func (sys *System) createNutStatsRecord(app core.App, collection *core.Collection, deviceKey string, device nut.NutData) error {
	record := core.NewRecord(collection)
	record.Set("system", sys.Id)
	record.Set("device", MakeStableHashId(sys.Id, deviceKey))
	record.Set("type", "1m")
	record.Set("battery_charge", device.BatteryCharge)
	record.Set("battery_runtime", device.BatteryRuntime)
	record.Set("battery_voltage", device.BatteryVoltage)
	record.Set("input_voltage", device.InputVoltage)
	record.Set("output_voltage", device.OutputVoltage)
	record.Set("load", device.Load)
	record.Set("output_current", device.OutputCurrent)
	record.Set("output_power", device.OutputPower)
	record.Set("output_power_source", device.OutputPowerSource)
	return app.SaveNoValidate(record)
}

func (sys *System) upsertNutDeviceRecord(app core.App, collection *core.Collection, deviceKey string, device nut.NutData, fresh bool) error {
	recordID := MakeStableHashId(sys.Id, deviceKey)

	record, err := app.FindRecordById(collection, recordID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		record = core.NewRecord(collection)
		record.Set("id", recordID)
	} else if !fresh {
		// Cached data from a failed refresh: keep the existing record as-is
		// instead of presenting stale values as a new observation.
		return nil
	}

	name := device.Model
	if name == "" {
		name = deviceKey
	}

	record.Set("system", sys.Id)
	record.Set("name", name)
	record.Set("model", device.Model)
	record.Set("manufacturer", device.Manufacturer)
	record.Set("serial", device.Serial)
	record.Set("firmware", device.Firmware)
	record.Set("driver", device.Driver)
	record.Set("device_type", device.DeviceType)
	record.Set("state", device.Health)
	record.Set("status", device.Status)
	record.Set("battery_charge", device.BatteryCharge)
	record.Set("battery_voltage", device.BatteryVoltage)
	record.Set("battery_runtime", device.BatteryRuntime)
	record.Set("input_voltage", device.InputVoltage)
	record.Set("output_voltage", device.OutputVoltage)
	record.Set("input_nominal", device.InputNominal)
	record.Set("load", device.Load)
	record.Set("output_current", device.OutputCurrent)
	record.Set("output_power", device.OutputPower)
	record.Set("output_power_source", device.OutputPowerSource)
	record.Set("outlets", device.Outlets)

	return app.SaveNoValidate(record)
}
