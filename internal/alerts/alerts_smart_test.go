//go:build testing
// +build testing

package alerts_test

import (
	"testing"
	"time"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
)

func TestSmartDeviceAlert(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system for the user
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	// Create a smart_device with state PASSED
	smartDevice, err := beszelTests.CreateRecord(hub, "smart_devices", map[string]any{
		"system": system.Id,
		"name":   "/dev/sda",
		"model":  "Samsung SSD 970 EVO",
		"state":  "PASSED",
	})
	assert.NoError(t, err)

	// Verify no emails sent initially
	assert.Zero(t, hub.TestMailer.TotalSend(), "should have 0 emails sent initially")

	// Re-fetch the record so PocketBase can properly track original values
	smartDevice, err = hub.FindRecordById("smart_devices", smartDevice.Id)
	assert.NoError(t, err)

	// Update the smart device state to FAILED
	smartDevice.Set("state", "FAILED")
	err = hub.Save(smartDevice)
	assert.NoError(t, err)

	// Wait for the alert to be processed
	time.Sleep(50 * time.Millisecond)

	// Verify that an email was sent
	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 email sent after state changed to FAILED")

	// Check the email content
	lastMessage := hub.TestMailer.LastMessage()
	assert.Contains(t, lastMessage.Subject, "SMART failure on test-system")
	assert.Contains(t, lastMessage.Subject, "/dev/sda")
	assert.Contains(t, lastMessage.Text, "Samsung SSD 970 EVO")
	assert.Contains(t, lastMessage.Text, "FAILED")
}

func TestSmartDeviceAlertNoAlertOnNonPassedToFailed(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system for the user
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	// Create a smart_device with state UNKNOWN
	smartDevice, err := beszelTests.CreateRecord(hub, "smart_devices", map[string]any{
		"system": system.Id,
		"name":   "/dev/sda",
		"model":  "Samsung SSD 970 EVO",
		"state":  "UNKNOWN",
	})
	assert.NoError(t, err)

	// Re-fetch the record so PocketBase can properly track original values
	smartDevice, err = hub.FindRecordById("smart_devices", smartDevice.Id)
	assert.NoError(t, err)

	// Update the state from UNKNOWN to FAILED - should NOT trigger alert
	smartDevice.Set("state", "FAILED")
	err = hub.Save(smartDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Verify no email was sent (only PASSED -> FAILED triggers alert)
	assert.Zero(t, hub.TestMailer.TotalSend(), "should have 0 emails when changing from UNKNOWN to FAILED")

	// Re-fetch the record again
	smartDevice, err = hub.FindRecordById("smart_devices", smartDevice.Id)
	assert.NoError(t, err)

	// Update state from FAILED to PASSED - should NOT trigger alert
	smartDevice.Set("state", "PASSED")
	err = hub.Save(smartDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Verify no email was sent
	assert.Zero(t, hub.TestMailer.TotalSend(), "should have 0 emails when changing from FAILED to PASSED")
}

func TestSmartDeviceAlertMultipleUsers(t *testing.T) {
	hub, user1 := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a second user
	user2, err := beszelTests.CreateUser(hub, "test2@example.com", "password")
	assert.NoError(t, err)

	// Create user settings for the second user
	_, err = beszelTests.CreateRecord(hub, "user_settings", map[string]any{
		"user":     user2.Id,
		"settings": `{"emails":["test2@example.com"],"webhooks":[]}`,
	})
	assert.NoError(t, err)

	// Create a system with both users
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "shared-system",
		"users": []string{user1.Id, user2.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	// Create a smart_device with state PASSED
	smartDevice, err := beszelTests.CreateRecord(hub, "smart_devices", map[string]any{
		"system": system.Id,
		"name":   "/dev/nvme0n1",
		"model":  "WD Black SN850",
		"state":  "PASSED",
	})
	assert.NoError(t, err)

	// Re-fetch the record so PocketBase can properly track original values
	smartDevice, err = hub.FindRecordById("smart_devices", smartDevice.Id)
	assert.NoError(t, err)

	// Update the smart device state to FAILED
	smartDevice.Set("state", "FAILED")
	err = hub.Save(smartDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Verify that two emails were sent (one for each user)
	assert.EqualValues(t, 2, hub.TestMailer.TotalSend(), "should have 2 emails sent for 2 users")
}

func TestSmartDeviceAlertWithoutModel(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a system for the user
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	assert.NoError(t, err)

	// Create a smart_device with state PASSED but no model
	smartDevice, err := beszelTests.CreateRecord(hub, "smart_devices", map[string]any{
		"system": system.Id,
		"name":   "/dev/sdb",
		"state":  "PASSED",
	})
	assert.NoError(t, err)

	// Re-fetch the record so PocketBase can properly track original values
	smartDevice, err = hub.FindRecordById("smart_devices", smartDevice.Id)
	assert.NoError(t, err)

	// Update the smart device state to FAILED
	smartDevice.Set("state", "FAILED")
	err = hub.Save(smartDevice)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Verify that an email was sent
	assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 email sent")

	// Check that the email doesn't have empty parentheses for missing model
	lastMessage := hub.TestMailer.LastMessage()
	assert.NotContains(t, lastMessage.Text, "()", "should not have empty parentheses for missing model")
	assert.Contains(t, lastMessage.Text, "/dev/sdb")
}
