//go:build testing

package systems

import (
	"errors"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/nut"
	"github.com/henrygd/beszel/internal/hub/expirymap"
	_ "github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordNutFetchResult(t *testing.T) {
	sm := &SystemManager{nutFetchMap: expirymap.New[nutFetchState](time.Hour)}
	t.Cleanup(sm.nutFetchMap.StopCleaner)

	sys := &System{
		Id:          "system-1",
		manager:     sm,
		nutInterval: time.Hour,
	}

	// Successful fetch with devices
	sys.recordNutFetchResult(nil, 3)
	state, ok := sm.nutFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected nut fetch result to be stored")
	assert.True(t, state.Successful, "expected successful fetch state to be recorded")

	// Failed fetch
	sys.recordNutFetchResult(errors.New("failed"), 0)
	state, ok = sm.nutFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected failed nut fetch state to be stored")
	assert.False(t, state.Successful, "expected failed nut fetch state to be marked unsuccessful")

	// Successful fetch but no devices
	sys.recordNutFetchResult(nil, 0)
	state, ok = sm.nutFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected fetch with zero devices to be stored")
	assert.False(t, state.Successful, "expected fetch with zero devices to be marked unsuccessful")
}

func TestShouldFetchNut(t *testing.T) {
	sm := &SystemManager{nutFetchMap: expirymap.New[nutFetchState](time.Hour)}
	t.Cleanup(sm.nutFetchMap.StopCleaner)

	sys := &System{
		Id:          "system-1",
		manager:     sm,
		nutInterval: time.Hour,
	}

	assert.True(t, sys.shouldFetchNut(), "expected initial nut fetch to be allowed")

	sys.recordNutFetchResult(errors.New("failed"), 0)
	assert.False(t, sys.shouldFetchNut(), "expected nut fetch to be blocked while interval entry exists")

	sm.nutFetchMap.Remove(sys.Id)
	assert.True(t, sys.shouldFetchNut(), "expected nut fetch to be allowed after interval entry is cleared")
}

func TestShouldFetchNut_IgnoresExtendedTTLWhenFetchIsDue(t *testing.T) {
	sm := &SystemManager{nutFetchMap: expirymap.New[nutFetchState](time.Hour)}
	t.Cleanup(sm.nutFetchMap.StopCleaner)

	sys := &System{
		Id:          "system-1",
		manager:     sm,
		nutInterval: time.Hour,
	}

	sm.nutFetchMap.Set(sys.Id, nutFetchState{
		LastAttempt: time.Now().Add(-2 * time.Hour).UnixMilli(),
		Successful:  true,
	}, 10*time.Minute)
	sm.nutFetchMap.UpdateExpiration(sys.Id, 3*time.Hour)

	assert.True(t, sys.shouldFetchNut(), "expected fetch time to take precedence over updated TTL")
}

func TestResetFailedNutFetchState(t *testing.T) {
	sm := &SystemManager{nutFetchMap: expirymap.New[nutFetchState](time.Hour)}
	t.Cleanup(sm.nutFetchMap.StopCleaner)

	sm.nutFetchMap.Set("system-1", nutFetchState{LastAttempt: time.Now().UnixMilli(), Successful: false}, time.Hour)
	sm.resetFailedNutFetchState("system-1")
	_, ok := sm.nutFetchMap.GetOk("system-1")
	assert.False(t, ok, "expected failed nut fetch state to be cleared on reconnect")

	sm.nutFetchMap.Set("system-1", nutFetchState{LastAttempt: time.Now().UnixMilli(), Successful: true}, time.Hour)
	sm.resetFailedNutFetchState("system-1")
	_, ok = sm.nutFetchMap.GetOk("system-1")
	assert.True(t, ok, "expected successful nut fetch state to be preserved")
}

func TestNutFetchInterval(t *testing.T) {
	sm := &SystemManager{nutFetchMap: expirymap.New[nutFetchState](time.Hour)}
	t.Cleanup(sm.nutFetchMap.StopCleaner)

	sys := &System{
		Id:      "system-1",
		manager: sm,
	}

	// Default interval when nutInterval is 0
	assert.Equal(t, time.Hour, sys.nutFetchInterval())

	// Custom interval
	sys.nutInterval = 30 * time.Second
	assert.Equal(t, 30*time.Second, sys.nutFetchInterval())
}

// countNutDeviceRecords returns the number of nut_devices rows for the given system.
func countNutDeviceRecords(t *testing.T, app core.App, systemID string) []*core.Record {
	t.Helper()
	records, err := app.FindAllRecords("nut_devices", nil)
	require.NoError(t, err)
	var forSystem []*core.Record
	for _, r := range records {
		if r.GetString("system") == systemID {
			forSystem = append(forSystem, r)
		}
	}
	return forSystem
}

func TestSaveNutDevices_RemovesStaleDevices(t *testing.T) {
	sys, testApp := newTestSystemWithHub(t)

	// First fetch reports two devices
	err := sys.saveNutDevices(map[string]nut.NutData{
		"ups1": {Model: "APC 1500", Health: nut.HealthOnline, BatteryCharge: 100},
		"ups2": {Model: "Eaton 9130", Health: nut.HealthOnline, BatteryCharge: 95},
	}, true)
	require.NoError(t, err)

	records := countNutDeviceRecords(t, testApp, sys.Id)
	require.Len(t, records, 2, "expected both devices to be saved")

	nutStats, err := testApp.FindAllRecords("nut_stats", nil)
	require.NoError(t, err)
	var statsForSystem []*core.Record
	for _, record := range nutStats {
		if record.GetString("system") == sys.Id {
			statsForSystem = append(statsForSystem, record)
		}
	}
	require.Len(t, statsForSystem, 2, "expected each device sample to be stored in history")
	charges := []float64{statsForSystem[0].GetFloat("battery_charge"), statsForSystem[1].GetFloat("battery_charge")}
	assert.ElementsMatch(t, []float64{100, 95}, charges)

	var record1 *core.Record
	for _, r := range records {
		if r.GetString("model") == "APC 1500" {
			record1 = r
		}
	}
	require.NotNil(t, record1, "expected to find device APC 1500")
	originalID := record1.Id

	deleteEvents := 0
	testApp.OnRecordAfterDeleteSuccess("nut_devices").BindFunc(func(e *core.RecordEvent) error {
		deleteEvents++
		return e.Next()
	})

	// Complete refresh confirms ups2 is gone
	err = sys.saveNutDevices(map[string]nut.NutData{
		"ups1": {Model: "APC 1500", Health: nut.HealthOnBattery, BatteryCharge: 80},
	}, true)
	require.NoError(t, err)

	records = countNutDeviceRecords(t, testApp, sys.Id)
	require.Len(t, records, 1, "expected stale device to be removed")
	assert.Equal(t, "APC 1500", records[0].GetString("model"))
	assert.Equal(t, originalID, records[0].Id, "expected existing device to be updated in place")
	assert.Equal(t, nut.HealthOnBattery, records[0].GetString("state"))
	assert.EqualValues(t, 80, records[0].GetFloat("battery_charge"))
	assert.Equal(t, 1, deleteEvents, "expected PocketBase delete hooks to run")
}

func TestSaveNutDevices_IncompleteDataDoesNotRemoveDevices(t *testing.T) {
	sys, testApp := newTestSystemWithHub(t)

	require.NoError(t, sys.saveNutDevices(map[string]nut.NutData{
		"ups1": {Model: "APC 1500", Health: nut.HealthOnline},
		"ups2": {Model: "Eaton 9130", Health: nut.HealthOnline},
	}, true))

	// Incomplete response: only ups1 collected, ups2 failed
	require.NoError(t, sys.saveNutDevices(map[string]nut.NutData{
		"ups1": {Model: "APC 1500", Health: nut.HealthOnBattery, BatteryCharge: 75},
	}, false))

	assert.Len(t, countNutDeviceRecords(t, testApp, sys.Id), 2, "incomplete data should not remove devices")
	record1, err := testApp.FindRecordById("nut_devices", MakeStableHashId(sys.Id, "ups1"))
	require.NoError(t, err)
	assert.Equal(t, nut.HealthOnBattery, record1.GetString("state"))
	assert.EqualValues(t, 75, record1.GetFloat("battery_charge"))
}

func TestSaveNutDevices_CompleteEmptyDataRemovesStaleDevices(t *testing.T) {
	sys, testApp := newTestSystemWithHub(t)

	err := sys.saveNutDevices(map[string]nut.NutData{
		"ups1": {Model: "APC 1500", Health: nut.HealthOnline},
	}, true)
	require.NoError(t, err)

	err = sys.saveNutDevices(map[string]nut.NutData{}, true)
	require.NoError(t, err)

	records := countNutDeviceRecords(t, testApp, sys.Id)
	assert.Empty(t, records, "complete empty fetch result should remove stale devices")
}

func TestSaveNutDevices_WithOutlets(t *testing.T) {
	sys, testApp := newTestSystemWithHub(t)

	outlets := []*nut.NutOutlet{
		{ID: "1", Desc: "Server 1", Status: "on", Power: 300, Current: 2.5, Voltage: 120.1},
		{ID: "2", Desc: "Server 2", Status: "off"},
	}

	err := sys.saveNutDevices(map[string]nut.NutData{
		"pdu1": {
			Model:      "APC PDU",
			Health:     nut.HealthOnline,
			DeviceType: nut.DeviceTypePDU,
			Outlets:    outlets,
		},
	}, true)
	require.NoError(t, err)

	records := countNutDeviceRecords(t, testApp, sys.Id)
	require.Len(t, records, 1)
	assert.Equal(t, nut.DeviceTypePDU, records[0].GetString("device_type"))

	// Verify outlets were stored
	var storedOutlets []*nut.NutOutlet
	require.NoError(t, records[0].UnmarshalJSONField("outlets", &storedOutlets))
	require.Len(t, storedOutlets, 2)
	assert.Equal(t, "1", storedOutlets[0].ID)
	assert.Equal(t, "on", storedOutlets[0].Status)
	assert.Equal(t, 300.0, storedOutlets[0].Power)
}
