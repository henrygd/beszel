//go:build testing

package systems

import (
	"errors"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/smart"
	esystem "github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/hub/expirymap"
	_ "github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// stubHub implements hubLike using a plain pocketbase test app, so
// smart-device DB tests can run in-package without an import cycle to
// internal/hub (which imports this package).
type stubHub struct{ core.App }

func (stubHub) GetSSHKey(dataDir string) (ssh.Signer, error) { return nil, nil }
func (stubHub) HandleSystemAlerts(systemRecord *core.Record, data *esystem.CombinedData) error {
	return nil
}
func (stubHub) HandleStatusAlerts(status string, systemRecord *core.Record) error { return nil }
func (stubHub) HandleContainerAlerts(systemRecord *core.Record, data *esystem.CombinedData, fetchLogs func(containerID string) (string, error)) error {
	return nil
}
func (stubHub) CancelPendingStatusAlerts(systemID string)    {}
func (stubHub) CancelPendingContainerAlerts(systemID string) {}

// newTestSystemWithHub creates a System backed by a real (temp) database, along
// with a matching "systems" record, for tests that need to exercise DB reads/writes.
func newTestSystemWithHub(t *testing.T) (*System, *pbtests.TestApp) {
	t.Helper()
	testApp, err := pbtests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(testApp.Cleanup)

	sm := &SystemManager{hub: stubHub{testApp}, smartFetchMap: expirymap.New[smartFetchState](time.Hour)}
	t.Cleanup(sm.smartFetchMap.StopCleaner)

	col, err := testApp.FindCachedCollectionByNameOrId("systems")
	require.NoError(t, err)
	systemRecord := core.NewRecord(col)
	systemRecord.Set("name", "test-system")
	systemRecord.Set("host", "127.0.0.1")
	systemRecord.Set("port", "45876")
	require.NoError(t, testApp.SaveNoValidate(systemRecord))

	sys := &System{Id: systemRecord.Id, manager: sm}
	return sys, testApp
}

func TestRecordSmartFetchResult(t *testing.T) {
	sm := &SystemManager{smartFetchMap: expirymap.New[smartFetchState](time.Hour)}
	t.Cleanup(sm.smartFetchMap.StopCleaner)

	sys := &System{
		Id:            "system-1",
		manager:       sm,
		smartInterval: time.Hour,
	}

	// Successful fetch with devices
	sys.recordSmartFetchResult(nil, 5)
	state, ok := sm.smartFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected smart fetch result to be stored")
	assert.True(t, state.Successful, "expected successful fetch state to be recorded")

	// Failed fetch
	sys.recordSmartFetchResult(errors.New("failed"), 0)
	state, ok = sm.smartFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected failed smart fetch state to be stored")
	assert.False(t, state.Successful, "expected failed smart fetch state to be marked unsuccessful")

	// Successful fetch but no devices
	sys.recordSmartFetchResult(nil, 0)
	state, ok = sm.smartFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected fetch with zero devices to be stored")
	assert.False(t, state.Successful, "expected fetch with zero devices to be marked unsuccessful")
}

func TestShouldFetchSmart(t *testing.T) {
	sm := &SystemManager{smartFetchMap: expirymap.New[smartFetchState](time.Hour)}
	t.Cleanup(sm.smartFetchMap.StopCleaner)

	sys := &System{
		Id:            "system-1",
		manager:       sm,
		smartInterval: time.Hour,
	}

	assert.True(t, sys.shouldFetchSmart(), "expected initial smart fetch to be allowed")

	sys.recordSmartFetchResult(errors.New("failed"), 0)
	assert.False(t, sys.shouldFetchSmart(), "expected smart fetch to be blocked while interval entry exists")

	sm.smartFetchMap.Remove(sys.Id)
	assert.True(t, sys.shouldFetchSmart(), "expected smart fetch to be allowed after interval entry is cleared")
}

func TestShouldFetchSmart_IgnoresExtendedTTLWhenFetchIsDue(t *testing.T) {
	sm := &SystemManager{smartFetchMap: expirymap.New[smartFetchState](time.Hour)}
	t.Cleanup(sm.smartFetchMap.StopCleaner)

	sys := &System{
		Id:            "system-1",
		manager:       sm,
		smartInterval: time.Hour,
	}

	sm.smartFetchMap.Set(sys.Id, smartFetchState{
		LastAttempt: time.Now().Add(-2 * time.Hour).UnixMilli(),
		Successful:  true,
	}, 10*time.Minute)
	sm.smartFetchMap.UpdateExpiration(sys.Id, 3*time.Hour)

	assert.True(t, sys.shouldFetchSmart(), "expected fetch time to take precedence over updated TTL")
}

func TestResetFailedSmartFetchState(t *testing.T) {
	sm := &SystemManager{smartFetchMap: expirymap.New[smartFetchState](time.Hour)}
	t.Cleanup(sm.smartFetchMap.StopCleaner)

	sm.smartFetchMap.Set("system-1", smartFetchState{LastAttempt: time.Now().UnixMilli(), Successful: false}, time.Hour)
	sm.resetFailedSmartFetchState("system-1")
	_, ok := sm.smartFetchMap.GetOk("system-1")
	assert.False(t, ok, "expected failed smart fetch state to be cleared on reconnect")

	sm.smartFetchMap.Set("system-1", smartFetchState{LastAttempt: time.Now().UnixMilli(), Successful: true}, time.Hour)
	sm.resetFailedSmartFetchState("system-1")
	_, ok = sm.smartFetchMap.GetOk("system-1")
	assert.True(t, ok, "expected successful smart fetch state to be preserved")
}

// countSmartDeviceRecords returns the number of smart_devices rows for the given system.
func countSmartDeviceRecords(t *testing.T, app core.App, systemID string) []*core.Record {
	t.Helper()
	records, err := app.FindAllRecords("smart_devices", nil)
	require.NoError(t, err)
	var forSystem []*core.Record
	for _, r := range records {
		if r.GetString("system") == systemID {
			forSystem = append(forSystem, r)
		}
	}
	return forSystem
}

func TestSaveSmartDevices_RemovesStaleDevices(t *testing.T) {
	sys, testApp := newTestSystemWithHub(t)

	// first fetch reports two devices: sda (serial AAA) and sdb (serial BBB)
	err := sys.saveSmartDevices(map[string]smart.SmartData{
		"AAA": {SerialNumber: "AAA", DiskName: "sda", ModelName: "Disk A"},
		"BBB": {SerialNumber: "BBB", DiskName: "sdb", ModelName: "Disk B"},
	}, true)
	require.NoError(t, err)

	records := countSmartDeviceRecords(t, testApp, sys.Id)
	require.Len(t, records, 2, "expected both devices to be saved")

	var recordA *core.Record
	for _, r := range records {
		if r.GetString("serial") == "AAA" {
			recordA = r
		}
	}
	require.NotNil(t, recordA, "expected to find device AAA")
	originalID := recordA.Id

	deleteEvents := 0
	testApp.OnRecordAfterDeleteSuccess("smart_devices").BindFunc(func(e *core.RecordEvent) error {
		deleteEvents++
		return e.Next()
	})

	// A complete refresh confirms that BBB is gone, so remove it through
	// PocketBase and notify realtime subscribers.
	err = sys.saveSmartDevices(map[string]smart.SmartData{
		"AAA": {SerialNumber: "AAA", DiskName: "sda", ModelName: "Disk A", Temperature: 42},
	}, true)
	require.NoError(t, err)

	records = countSmartDeviceRecords(t, testApp, sys.Id)
	require.Len(t, records, 1, "expected stale device BBB to be removed")
	assert.Equal(t, "AAA", records[0].GetString("serial"))
	assert.Equal(t, originalID, records[0].Id, "expected existing device to be updated in place, not recreated")
	assert.EqualValues(t, 42, records[0].GetInt("temp"))
	assert.Equal(t, 1, deleteEvents, "expected PocketBase delete hooks to run")
}

func TestSaveSmartDevices_IncompleteDataDoesNotRemoveDevices(t *testing.T) {
	sys, testApp := newTestSystemWithHub(t)

	require.NoError(t, sys.saveSmartDevices(map[string]smart.SmartData{
		"AAA": {SerialNumber: "AAA", DiskName: "sda"},
		"BBB": {SerialNumber: "BBB", DiskName: "sdb"},
	}, true))

	// AAA was collected but BBB failed. The response is useful for updating AAA,
	// but it is not authoritative enough to remove BBB.
	require.NoError(t, sys.saveSmartDevices(map[string]smart.SmartData{
		"AAA": {SerialNumber: "AAA", DiskName: "sda", Temperature: 42},
	}, false))

	assert.Len(t, countSmartDeviceRecords(t, testApp, sys.Id), 2)
	recordA, err := testApp.FindRecordById("smart_devices", makeStableHashId(sys.Id, "AAA"))
	require.NoError(t, err)
	assert.EqualValues(t, 42, recordA.GetInt("temp"))
}

func TestSaveSmartDevices_EmptyDataIsNoop(t *testing.T) {
	sys, testApp := newTestSystemWithHub(t)

	err := sys.saveSmartDevices(map[string]smart.SmartData{
		"AAA": {SerialNumber: "AAA", DiskName: "sda"},
	}, true)
	require.NoError(t, err)

	err = sys.saveSmartDevices(map[string]smart.SmartData{}, true)
	require.NoError(t, err)

	records := countSmartDeviceRecords(t, testApp, sys.Id)
	assert.Len(t, records, 1, "empty fetch result should not delete existing devices")
}
