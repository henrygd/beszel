//go:build testing

package alerts_test

import (
	"testing"
	"time"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
)

func TestNutDeviceAlert(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	nutDevice, err := beszelTests.CreateRecord(hub, "nut_devices", map[string]any{
		"system": system.Id,
		"name":   "ups1",
		"model":  "APC SMT 1500",
		"state":  "ONLINE",
	})
	assert.NoError(t, err)

	assert.Zero(t, hub.TestMailer.TotalSend(), "should have 0 emails sent initially")

	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	nutDevice.Set("state", "ON_BATTERY")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 email sent after state changed to ON_BATTERY")
	lastMessage := hub.TestMailer.LastMessage()
	assert.Contains(t, lastMessage.Subject, "NUT on battery on test-system")
	assert.Contains(t, lastMessage.Subject, "ups1")
	assert.Contains(t, lastMessage.Text, "APC SMT 1500")
	assert.Contains(t, lastMessage.Text, "ON_BATTERY")
}

func TestNutDeviceAlertOnlineToFault(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	nutDevice, err := beszelTests.CreateRecord(hub, "nut_devices", map[string]any{
		"system": system.Id,
		"name":   "ups1",
		"model":  "APC SMT 1500",
		"state":  "ONLINE",
	})
	assert.NoError(t, err)

	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	nutDevice.Set("state", "FAULT")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 email sent after state changed to FAULT")
	lastMessage := hub.TestMailer.LastMessage()
	assert.Contains(t, lastMessage.Subject, "NUT fault on test-system")
	assert.Contains(t, lastMessage.Text, "FAULT")
}

func TestNutDeviceAlertOnlineToLowBattery(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	nutDevice, err := beszelTests.CreateRecord(hub, "nut_devices", map[string]any{
		"system": system.Id,
		"name":   "ups1",
		"model":  "APC SMT 1500",
		"state":  "ONLINE",
	})
	assert.NoError(t, err)

	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	nutDevice.Set("state", "LOW_BATTERY")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 email sent after state changed to LOW_BATTERY")
	lastMessage := hub.TestMailer.LastMessage()
	assert.Contains(t, lastMessage.Subject, "NUT low battery on test-system")
	assert.Contains(t, lastMessage.Text, "LOW_BATTERY")
}

func TestNutDeviceAlertOnBatteryToLowBattery(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	nutDevice, err := beszelTests.CreateRecord(hub, "nut_devices", map[string]any{
		"system": system.Id,
		"name":   "ups1",
		"model":  "APC SMT 1500",
		"state":  "ON_BATTERY",
	})
	assert.NoError(t, err)

	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	nutDevice.Set("state", "LOW_BATTERY")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 email sent after state changed from ON_BATTERY to LOW_BATTERY")
	lastMessage := hub.TestMailer.LastMessage()
	assert.Contains(t, lastMessage.Subject, "NUT low battery on test-system")
}

func TestNutDeviceAlertNoAlertOnUnknownToFault(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	nutDevice, err := beszelTests.CreateRecord(hub, "nut_devices", map[string]any{
		"system": system.Id,
		"name":   "ups1",
		"model":  "APC SMT 1500",
		"state":  "UNKNOWN",
	})
	assert.NoError(t, err)

	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	// UNKNOWN -> FAULT should NOT trigger alert (unknown has severity 0)
	nutDevice.Set("state", "FAULT")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.Zero(t, hub.TestMailer.TotalSend(), "should have 0 emails when changing from UNKNOWN to FAULT")

	// FAULT -> ONLINE should NOT trigger alert (recovery)
	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	nutDevice.Set("state", "ONLINE")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.Zero(t, hub.TestMailer.TotalSend(), "should have 0 emails when recovering from FAULT to ONLINE")
}

func TestNutDeviceAlertNoAlertOnSameState(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	nutDevice, err := beszelTests.CreateRecord(hub, "nut_devices", map[string]any{
		"system": system.Id,
		"name":   "ups1",
		"model":  "APC SMT 1500",
		"state":  "ONLINE",
	})
	assert.NoError(t, err)

	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	// Same state should not trigger
	nutDevice.Set("state", "ONLINE")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.Zero(t, hub.TestMailer.TotalSend(), "should have 0 emails when state stays the same")
}

func TestNutDeviceAlertMultipleUsers(t *testing.T) {
	hub, user1 := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	user2, err := beszelTests.CreateUser(hub, "test2@example.com", "password")
	assert.NoError(t, err)

	_, err = beszelTests.CreateRecord(hub, "user_settings", map[string]any{
		"user":     user2.Id,
		"settings": `{"emails":["test2@example.com"],"webhooks":[]}`,
	})
	assert.NoError(t, err)

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "shared-system",
		"users": []string{user1.Id, user2.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	nutDevice, err := beszelTests.CreateRecord(hub, "nut_devices", map[string]any{
		"system": system.Id,
		"name":   "ups1",
		"model":  "APC SMT 1500",
		"state":  "ONLINE",
	})
	assert.NoError(t, err)

	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	nutDevice.Set("state", "FAULT")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 2, hub.TestMailer.TotalSend(), "should have 2 emails sent for 2 users")
}

func TestNutDeviceAlertWithoutModel(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	nutDevice, err := beszelTests.CreateRecord(hub, "nut_devices", map[string]any{
		"system": system.Id,
		"name":   "ups1",
		"state":  "ONLINE",
	})
	assert.NoError(t, err)

	nutDevice, err = hub.FindRecordById("nut_devices", nutDevice.Id)
	assert.NoError(t, err)

	nutDevice.Set("state", "FAULT")
	err = hub.Save(nutDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 email sent")
	lastMessage := hub.TestMailer.LastMessage()
	assert.NotContains(t, lastMessage.Text, "()", "should not have empty parentheses for missing model")
	assert.Contains(t, lastMessage.Text, "ups1")
}