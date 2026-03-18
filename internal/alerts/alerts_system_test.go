//go:build testing

package alerts_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type systemAlertValueSetter[T any] func(info *system.Info, stats *system.Stats, value T)

type systemAlertTestFixture struct {
	hub     *beszelTests.TestHub
	alertID string
	submit  func(*system.CombinedData) error
}

func createCombinedData[T any](value T, setValue systemAlertValueSetter[T]) *system.CombinedData {
	var data system.CombinedData
	setValue(&data.Info, &data.Stats, value)
	return &data
}

func newSystemAlertTestFixture(t *testing.T, alertName string, min int, threshold float64) *systemAlertTestFixture {
	t.Helper()

	hub, user := beszelTests.GetHubWithUser(t)

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	sysManagerSystem, err := hub.GetSystemManager().GetSystemFromStore(systemRecord.Id)
	require.NoError(t, err)
	require.NotNil(t, sysManagerSystem)
	sysManagerSystem.StopUpdater()

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	alertRecord, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   alertName,
		"system": systemRecord.Id,
		"user":   user.Id,
		"min":    min,
		"value":  threshold,
	})
	require.NoError(t, err)

	assert.False(t, alertRecord.GetBool("triggered"), "Alert should not be triggered initially")

	alertsCache := hub.GetAlertManager().GetSystemAlertsCache()
	cachedAlerts := alertsCache.GetAlertsExcludingNames(systemRecord.Id, "Status")
	assert.Len(t, cachedAlerts, 1, "Alert should be in cache")

	return &systemAlertTestFixture{
		hub:     hub,
		alertID: alertRecord.Id,
		submit: func(data *system.CombinedData) error {
			_, err := sysManagerSystem.CreateRecords(data)
			return err
		},
	}
}

func (fixture *systemAlertTestFixture) cleanup() {
	fixture.hub.Cleanup()
}

func submitValue[T any](fixture *systemAlertTestFixture, t *testing.T, value T, setValue systemAlertValueSetter[T]) {
	t.Helper()
	require.NoError(t, fixture.submit(createCombinedData(value, setValue)))
}

func (fixture *systemAlertTestFixture) assertTriggered(t *testing.T, triggered bool, message string) {
	t.Helper()

	alertRecord, err := fixture.hub.FindRecordById("alerts", fixture.alertID)
	require.NoError(t, err)
	assert.Equal(t, triggered, alertRecord.GetBool("triggered"), message)
}

func waitForSystemAlert(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

func testOneMinuteSystemAlert[T any](t *testing.T, alertName string, threshold float64, setValue systemAlertValueSetter[T], triggerValue, resolveValue T) {
	t.Helper()

	synctest.Test(t, func(t *testing.T) {
		fixture := newSystemAlertTestFixture(t, alertName, 1, threshold)
		defer fixture.cleanup()

		submitValue(fixture, t, triggerValue, setValue)
		waitForSystemAlert(time.Second)

		fixture.assertTriggered(t, true, "Alert should be triggered")
		assert.Equal(t, 1, fixture.hub.TestMailer.TotalSend(), "An email should have been sent")

		submitValue(fixture, t, resolveValue, setValue)
		waitForSystemAlert(time.Second)

		fixture.assertTriggered(t, false, "Alert should be untriggered")
		assert.Equal(t, 2, fixture.hub.TestMailer.TotalSend(), "A second email should have been sent for untriggering the alert")

		waitForSystemAlert(time.Minute)
	})
}

func testMultiMinuteSystemAlert[T any](t *testing.T, alertName string, threshold float64, min int, setValue systemAlertValueSetter[T], baselineValue, triggerValue, resolveValue T) {
	t.Helper()

	synctest.Test(t, func(t *testing.T) {
		fixture := newSystemAlertTestFixture(t, alertName, min, threshold)
		defer fixture.cleanup()

		submitValue(fixture, t, baselineValue, setValue)
		waitForSystemAlert(time.Minute + time.Second)
		fixture.assertTriggered(t, false, "Alert should not be triggered yet")

		submitValue(fixture, t, triggerValue, setValue)
		waitForSystemAlert(time.Minute)
		fixture.assertTriggered(t, false, "Alert should not be triggered until the history window is full")

		submitValue(fixture, t, triggerValue, setValue)
		waitForSystemAlert(time.Second)
		fixture.assertTriggered(t, true, "Alert should be triggered")
		assert.Equal(t, 1, fixture.hub.TestMailer.TotalSend(), "An email should have been sent")

		submitValue(fixture, t, resolveValue, setValue)
		waitForSystemAlert(time.Second)
		fixture.assertTriggered(t, false, "Alert should be untriggered")
		assert.Equal(t, 2, fixture.hub.TestMailer.TotalSend(), "A second email should have been sent for untriggering the alert")
	})
}

func setCPUAlertValue(info *system.Info, stats *system.Stats, value float64) {
	info.Cpu = value
	stats.Cpu = value
}

func setMemoryAlertValue(info *system.Info, stats *system.Stats, value float64) {
	info.MemPct = value
	stats.MemPct = value
}

func setDiskAlertValue(info *system.Info, stats *system.Stats, value float64) {
	info.DiskPct = value
	stats.DiskPct = value
}

func setBandwidthAlertValue(info *system.Info, stats *system.Stats, value [2]uint64) {
	info.BandwidthBytes = value[0] + value[1]
	stats.Bandwidth = value
}

func megabytesToBytes(mb uint64) uint64 {
	return mb * 1024 * 1024
}

func setGPUAlertValue(info *system.Info, stats *system.Stats, value float64) {
	info.GpuPct = value
	stats.GPUData = map[string]system.GPUData{
		"GPU0": {Usage: value},
	}
}

func setTemperatureAlertValue(info *system.Info, stats *system.Stats, value float64) {
	info.DashboardTemp = value
	stats.Temperatures = map[string]float64{
		"Temp0": value,
	}
}

func setLoadAvgAlertValue(info *system.Info, stats *system.Stats, value [3]float64) {
	info.LoadAvg = value
	stats.LoadAvg = value
}

func setBatteryAlertValue(info *system.Info, stats *system.Stats, value [2]uint8) {
	info.Battery = value
	stats.Battery = value
}

func TestSystemAlertsOneMin(t *testing.T) {
	testOneMinuteSystemAlert(t, "CPU", 50, setCPUAlertValue, 51, 49)
	testOneMinuteSystemAlert(t, "Memory", 50, setMemoryAlertValue, 51, 49)
	testOneMinuteSystemAlert(t, "Disk", 50, setDiskAlertValue, 51, 49)
	testOneMinuteSystemAlert(t, "Bandwidth", 50, setBandwidthAlertValue, [2]uint64{megabytesToBytes(26), megabytesToBytes(25)}, [2]uint64{megabytesToBytes(25), megabytesToBytes(24)})
	testOneMinuteSystemAlert(t, "GPU", 50, setGPUAlertValue, 51, 49)
	testOneMinuteSystemAlert(t, "Temperature", 70, setTemperatureAlertValue, 71, 69)
	testOneMinuteSystemAlert(t, "LoadAvg1", 4, setLoadAvgAlertValue, [3]float64{4.1, 0, 0}, [3]float64{3.9, 0, 0})
	testOneMinuteSystemAlert(t, "LoadAvg5", 4, setLoadAvgAlertValue, [3]float64{0, 4.1, 0}, [3]float64{0, 3.9, 0})
	testOneMinuteSystemAlert(t, "LoadAvg15", 4, setLoadAvgAlertValue, [3]float64{0, 0, 4.1}, [3]float64{0, 0, 3.9})
	testOneMinuteSystemAlert(t, "Battery", 20, setBatteryAlertValue, [2]uint8{19, 0}, [2]uint8{21, 0})
}

func TestSystemAlertsTwoMin(t *testing.T) {
	testMultiMinuteSystemAlert(t, "CPU", 50, 2, setCPUAlertValue, 10, 51, 48)
	testMultiMinuteSystemAlert(t, "Memory", 50, 2, setMemoryAlertValue, 10, 51, 48)
	testMultiMinuteSystemAlert(t, "Disk", 50, 2, setDiskAlertValue, 10, 51, 48)
	testMultiMinuteSystemAlert(t, "Bandwidth", 50, 2, setBandwidthAlertValue, [2]uint64{megabytesToBytes(10), megabytesToBytes(10)}, [2]uint64{megabytesToBytes(26), megabytesToBytes(25)}, [2]uint64{megabytesToBytes(10), megabytesToBytes(10)})
	testMultiMinuteSystemAlert(t, "GPU", 50, 2, setGPUAlertValue, 10, 51, 48)
	testMultiMinuteSystemAlert(t, "Temperature", 70, 2, setTemperatureAlertValue, 10, 71, 67)
	testMultiMinuteSystemAlert(t, "LoadAvg1", 4, 2, setLoadAvgAlertValue, [3]float64{0, 0, 0}, [3]float64{4.1, 0, 0}, [3]float64{3.5, 0, 0})
	testMultiMinuteSystemAlert(t, "LoadAvg5", 4, 2, setLoadAvgAlertValue, [3]float64{0, 2, 0}, [3]float64{0, 4.1, 0}, [3]float64{0, 3.5, 0})
	testMultiMinuteSystemAlert(t, "LoadAvg15", 4, 2, setLoadAvgAlertValue, [3]float64{0, 0, 2}, [3]float64{0, 0, 4.1}, [3]float64{0, 0, 3.5})
	testMultiMinuteSystemAlert(t, "Battery", 20, 2, setBatteryAlertValue, [2]uint8{21, 0}, [2]uint8{19, 0}, [2]uint8{25, 1})
}
