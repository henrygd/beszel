//go:build testing

package alerts_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
)

// marshal to json and return an io.Reader (for use in ApiScenario.Body)
func jsonReader(v any) io.Reader {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(data)
}

func TestUserAlertsApi(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	hub.StartHub()

	user1, _ := beszelTests.CreateUser(hub, "alertstest@example.com", "password")
	user1Token, _ := user1.NewAuthToken()

	user2, _ := beszelTests.CreateUser(hub, "alertstest2@example.com", "password")
	user2Token, _ := user2.NewAuthToken()

	system1, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system1",
		"users": []string{user1.Id},
		"host":  "127.0.0.1",
	})

	system2, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system2",
		"users": []string{user1.Id, user2.Id},
		"host":  "127.0.0.2",
	})

	userRecords, _ := hub.CountRecords("users")
	assert.EqualValues(t, 2, userRecords, "all users should be created")

	systemRecords, _ := hub.CountRecords("systems")
	assert.EqualValues(t, 2, systemRecords, "all systems should be created")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		// {
		// 	Name:            "GET not implemented - returns index",
		// 	Method:          http.MethodGet,
		// 	URL:             "/api/beszel/user-alerts",
		// 	ExpectedStatus:  200,
		// 	ExpectedContent: []string{"<html ", "globalThis.BESZEL"},
		// 	TestAppFactory:  testAppFactory,
		// },
		{
			Name:            "POST no auth",
			Method:          http.MethodPost,
			URL:             "/api/beszel/user-alerts",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST no body",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Bad data"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST bad data",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Bad data"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"invalidField": "this should cause validation error",
				"threshold":    "not a number",
			}),
		},
		{
			Name:   "POST malformed JSON",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Bad data"},
			TestAppFactory:  testAppFactory,
			Body:            strings.NewReader(`{"alertType": "cpu", "threshold": 80, "enabled": true,}`),
		},
		{
			Name:   "POST valid alert data multiple systems",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":      "CPU",
				"value":     69,
				"min":       9,
				"systems":   []string{system1.Id, system2.Id},
				"overwrite": false,
			}),
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				// check total alerts
				alerts, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 2, alerts, "should have 2 alerts")
				// check alert has correct values
				matchingAlerts, _ := app.CountRecords("alerts", dbx.HashExp{"name": "CPU", "user": user1.Id, "system": system1.Id, "value": 69, "min": 9})
				assert.EqualValues(t, 1, matchingAlerts, "should have 1 alert")
			},
		},
		{
			Name:   "POST valid alert data single system",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "Memory",
				"systems": []string{system1.Id},
				"value":   90,
				"min":     10,
			}),
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				user1Alerts, _ := app.CountRecords("alerts", dbx.HashExp{"user": user1.Id})
				assert.EqualValues(t, 3, user1Alerts, "should have 3 alerts")
			},
		},
		{
			Name:   "Overwrite: false, should not overwrite existing alert",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":      "CPU",
				"value":     45,
				"min":       5,
				"systems":   []string{system1.Id},
				"overwrite": false,
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.ClearCollection(t, app, "alerts")
				beszelTests.CreateRecord(app, "alerts", map[string]any{
					"name":   "CPU",
					"system": system1.Id,
					"user":   user1.Id,
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alerts, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 1, alerts, "should have 1 alert")
				alert, _ := app.FindFirstRecordByFilter("alerts", "name = 'CPU' && user = {:user}", dbx.Params{"user": user1.Id})
				assert.EqualValues(t, 80, alert.Get("value"), "should have 80 as value")
			},
		},
		{
			Name:   "Overwrite: true, should overwrite existing alert",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user2Token,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":      "CPU",
				"value":     45,
				"min":       5,
				"systems":   []string{system2.Id},
				"overwrite": true,
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.ClearCollection(t, app, "alerts")
				beszelTests.CreateRecord(app, "alerts", map[string]any{
					"name":   "CPU",
					"system": system2.Id,
					"user":   user2.Id,
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alerts, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 1, alerts, "should have 1 alert")
				alert, _ := app.FindFirstRecordByFilter("alerts", "name = 'CPU' && user = {:user}", dbx.Params{"user": user2.Id})
				assert.EqualValues(t, 45, alert.Get("value"), "should have 45 as value")
			},
		},
		{
			Name:            "DELETE no auth",
			Method:          http.MethodDelete,
			URL:             "/api/beszel/user-alerts",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"systems": []string{system1.Id},
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.ClearCollection(t, app, "alerts")
				beszelTests.CreateRecord(app, "alerts", map[string]any{
					"name":   "CPU",
					"system": system1.Id,
					"user":   user1.Id,
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alerts, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 1, alerts, "should have 1 alert")
			},
		},
		{
			Name:   "DELETE alert",
			Method: http.MethodDelete,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"count\":1", "\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"systems": []string{system1.Id},
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.ClearCollection(t, app, "alerts")
				beszelTests.CreateRecord(app, "alerts", map[string]any{
					"name":   "CPU",
					"system": system1.Id,
					"user":   user1.Id,
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alerts, _ := app.CountRecords("alerts")
				assert.Zero(t, alerts, "should have 0 alerts")
			},
		},
		{
			Name:   "DELETE alert multiple systems",
			Method: http.MethodDelete,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"count\":2", "\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "Memory",
				"systems": []string{system1.Id, system2.Id},
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.ClearCollection(t, app, "alerts")
				for _, systemId := range []string{system1.Id, system2.Id} {
					_, err := beszelTests.CreateRecord(app, "alerts", map[string]any{
						"name":   "Memory",
						"system": systemId,
						"user":   user1.Id,
						"value":  90,
						"min":    10,
					})
					assert.NoError(t, err, "should create alert")
				}
				alerts, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 2, alerts, "should have 2 alerts")
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alerts, _ := app.CountRecords("alerts")
				assert.Zero(t, alerts, "should have 0 alerts")
			},
		},
		{
			Name:   "User 2 should not be able to delete alert of user 1",
			Method: http.MethodDelete,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": user2Token,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"count\":1", "\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"systems": []string{system2.Id},
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.ClearCollection(t, app, "alerts")
				for _, user := range []string{user1.Id, user2.Id} {
					beszelTests.CreateRecord(app, "alerts", map[string]any{
						"name":   "CPU",
						"system": system2.Id,
						"user":   user,
						"value":  80,
						"min":    10,
					})
				}
				alerts, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 2, alerts, "should have 2 alerts")
				user1AlertCount, _ := app.CountRecords("alerts", dbx.HashExp{"user": user1.Id})
				assert.EqualValues(t, 1, user1AlertCount, "should have 1 alert")
				user2AlertCount, _ := app.CountRecords("alerts", dbx.HashExp{"user": user2.Id})
				assert.EqualValues(t, 1, user2AlertCount, "should have 1 alert")
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				user1AlertCount, _ := app.CountRecords("alerts", dbx.HashExp{"user": user1.Id})
				assert.EqualValues(t, 1, user1AlertCount, "should have 1 alert")
				user2AlertCount, _ := app.CountRecords("alerts", dbx.HashExp{"user": user2.Id})
				assert.Zero(t, user2AlertCount, "should have 0 alerts")
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestStatusAlerts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, user := beszelTests.GetHubWithUser(t)
		defer hub.Cleanup()

		systems, err := beszelTests.CreateSystems(hub, 4, user.Id, "paused")
		assert.NoError(t, err)

		var alerts []*core.Record
		for i, system := range systems {
			alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
				"name":   "Status",
				"system": system.Id,
				"user":   user.Id,
				"min":    i + 1,
			})
			assert.NoError(t, err)
			alerts = append(alerts, alert)
		}

		time.Sleep(10 * time.Millisecond)

		for _, alert := range alerts {
			assert.False(t, alert.GetBool("triggered"), "Alert should not be triggered immediately")
		}
		if hub.TestMailer.TotalSend() != 0 {
			assert.Zero(t, hub.TestMailer.TotalSend(), "Expected 0 messages, got %d", hub.TestMailer.TotalSend())
		}
		for _, system := range systems {
			assert.EqualValues(t, "paused", system.GetString("status"), "System should be paused")
		}
		for _, system := range systems {
			system.Set("status", "up")
			err = hub.SaveNoValidate(system)
			assert.NoError(t, err)
		}
		time.Sleep(time.Second)
		assert.EqualValues(t, 0, hub.GetPendingAlertsCount(), "should have 0 alerts in the pendingAlerts map")
		for _, system := range systems {
			system.Set("status", "down")
			err = hub.SaveNoValidate(system)
			assert.NoError(t, err)
		}
		// after 30 seconds, should have 4 alerts in the pendingAlerts map, no triggered alerts
		time.Sleep(time.Second * 30)
		assert.EqualValues(t, 4, hub.GetPendingAlertsCount(), "should have 4 alerts in the pendingAlerts map")
		triggeredCount, err := hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
		assert.NoError(t, err)
		assert.EqualValues(t, 0, triggeredCount, "should have 0 alert triggered")
		assert.EqualValues(t, 0, hub.TestMailer.TotalSend(), "should have 0 messages sent")
		// after 1:30 seconds, should have 1 triggered alert and 3 pending alerts
		time.Sleep(time.Second * 60)
		assert.EqualValues(t, 3, hub.GetPendingAlertsCount(), "should have 3 alerts in the pendingAlerts map")
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, triggeredCount, "should have 1 alert triggered")
		assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 messages sent")
		// after 2:30 seconds, should have 2 triggered alerts and 2 pending alerts
		time.Sleep(time.Second * 60)
		assert.EqualValues(t, 2, hub.GetPendingAlertsCount(), "should have 2 alerts in the pendingAlerts map")
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
		assert.NoError(t, err)
		assert.EqualValues(t, 2, triggeredCount, "should have 2 alert triggered")
		assert.EqualValues(t, 2, hub.TestMailer.TotalSend(), "should have 2 messages sent")
		// now we will bring the remaning systems back up
		for _, system := range systems {
			system.Set("status", "up")
			err = hub.SaveNoValidate(system)
			assert.NoError(t, err)
		}
		time.Sleep(time.Second)
		// should have 0 alerts in the pendingAlerts map and 0 alerts triggered
		assert.EqualValues(t, 0, hub.GetPendingAlertsCount(), "should have 0 alerts in the pendingAlerts map")
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
		assert.NoError(t, err)
		assert.Zero(t, triggeredCount, "should have 0 alert triggered")
		// 4 messages sent, 2 down alerts and 2 up alerts for first 2 systems
		assert.EqualValues(t, 4, hub.TestMailer.TotalSend(), "should have 4 messages sent")
	})
}

func TestStatusAlertDismissedWhileDownDoesNotSendUpNotification(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, user := beszelTests.GetHubWithUser(t)
		defer hub.Cleanup()

		system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
			"name":   "dismiss-test-system",
			"users":  []string{user.Id},
			"host":   "127.0.0.1",
			"status": "paused",
		})
		assert.NoError(t, err)

		alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name":   "Status",
			"system": system.Id,
			"user":   user.Id,
			"min":    1,
		})
		assert.NoError(t, err)

		system.Set("status", "up")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)

		time.Sleep(time.Second)

		system.Set("status", "down")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)

		// Wait long enough for the down alert to trigger.
		time.Sleep(75 * time.Second)

		assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should have 1 down notification")
		alert, err = hub.FindRecordById("alerts", alert.Id)
		assert.NoError(t, err)
		assert.True(t, alert.GetBool("triggered"), "status alert should be triggered before dismissal")

		// Simulate manual dismissal from the UI while the system is still down.
		alert.Set("triggered", false)
		err = hub.SaveNoValidate(alert)
		assert.NoError(t, err)

		system.Set("status", "up")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)

		time.Sleep(time.Second)

		assert.EqualValues(t, 1, hub.TestMailer.TotalSend(), "should not send an up notification after dismissal")
	})
}

func TestAlertsHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hub, user := beszelTests.GetHubWithUser(t)
		defer hub.Cleanup()

		// Create systems and alerts
		systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
		assert.NoError(t, err)
		system := systems[0]

		alert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name":   "Status",
			"system": system.Id,
			"user":   user.Id,
			"min":    1,
		})
		assert.NoError(t, err)

		// Initially, no alert history records should exist
		initialHistoryCount, err := hub.CountRecords("alerts_history", nil)
		assert.NoError(t, err)
		assert.Zero(t, initialHistoryCount, "Should have 0 alert history records initially")

		// Set system to up initially
		system.Set("status", "up")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)
		time.Sleep(10 * time.Millisecond)

		// Set system to down to trigger alert
		system.Set("status", "down")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)

		// Wait for alert to trigger (after the downtime delay)
		// With 1 minute delay, we need to wait at least 1 minute + some buffer
		time.Sleep(time.Second * 75)

		// Check that alert is triggered
		triggeredCount, err := hub.CountRecords("alerts", dbx.HashExp{"triggered": true, "id": alert.Id})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, triggeredCount, "Alert should be triggered")

		// Check that alert history record was created
		historyCount, err := hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert.Id})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, historyCount, "Should have 1 alert history record for triggered alert")

		// Get the alert history record and verify it's not resolved immediately
		historyRecord, err := hub.FindFirstRecordByFilter("alerts_history", "alert_id={:alert_id}", dbx.Params{"alert_id": alert.Id})
		assert.NoError(t, err)
		assert.NotNil(t, historyRecord, "Alert history record should exist")
		assert.Equal(t, alert.Id, historyRecord.GetString("alert_id"), "Alert history should reference correct alert")
		assert.Equal(t, system.Id, historyRecord.GetString("system"), "Alert history should reference correct system")
		assert.Equal(t, "Status", historyRecord.GetString("name"), "Alert history should have correct name")

		// The alert history might be resolved immediately in some cases, so let's check the alert's triggered status
		alertRecord, err := hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alert.Id})
		assert.NoError(t, err)
		assert.True(t, alertRecord.GetBool("triggered"), "Alert should still be triggered when checking history")

		// Now resolve the alert by setting system back to up
		system.Set("status", "up")
		err = hub.SaveNoValidate(system)
		assert.NoError(t, err)
		time.Sleep(200 * time.Millisecond)

		// Check that alert is no longer triggered
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true, "id": alert.Id})
		assert.NoError(t, err)
		assert.Zero(t, triggeredCount, "Alert should not be triggered after system is back up")

		// Check that alert history record is now resolved
		historyRecord, err = hub.FindFirstRecordByFilter("alerts_history", "alert_id={:alert_id}", dbx.Params{"alert_id": alert.Id})
		assert.NoError(t, err)
		assert.NotNil(t, historyRecord, "Alert history record should still exist")
		assert.NotNil(t, historyRecord.Get("resolved"), "Alert history should be resolved")

		// Test deleting a triggered alert resolves its history
		// Create another system and alert
		systems2, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
		assert.NoError(t, err)
		system2 := systems2[0]
		system2.Set("name", "test-system-2") // Rename for clarity
		err = hub.SaveNoValidate(system2)
		assert.NoError(t, err)

		alert2, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
			"name":   "Status",
			"system": system2.Id,
			"user":   user.Id,
			"min":    1,
		})
		assert.NoError(t, err)

		// Set system2 to down to trigger alert
		system2.Set("status", "down")
		err = hub.SaveNoValidate(system2)
		assert.NoError(t, err)

		// Wait for alert to trigger
		time.Sleep(time.Second * 75)

		// Verify alert is triggered and history record exists
		triggeredCount, err = hub.CountRecords("alerts", dbx.HashExp{"triggered": true, "id": alert2.Id})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, triggeredCount, "Second alert should be triggered")

		historyCount, err = hub.CountRecords("alerts_history", dbx.HashExp{"alert_id": alert2.Id})
		assert.NoError(t, err)
		assert.EqualValues(t, 1, historyCount, "Should have 1 alert history record for second alert")

		// Delete the triggered alert
		err = hub.Delete(alert2)
		assert.NoError(t, err)

		// Check that alert history record is resolved after deletion
		historyRecord2, err := hub.FindFirstRecordByFilter("alerts_history", "alert_id={:alert_id}", dbx.Params{"alert_id": alert2.Id})
		assert.NoError(t, err)
		assert.NotNil(t, historyRecord2, "Alert history record should still exist after alert deletion")
		assert.NotNil(t, historyRecord2.Get("resolved"), "Alert history should be resolved after alert deletion")

		// Verify total history count is correct (2 records total)
		totalHistoryCount, err := hub.CountRecords("alerts_history", nil)
		assert.NoError(t, err)
		assert.EqualValues(t, 2, totalHistoryCount, "Should have 2 total alert history records")
	})
}
func TestResolveStatusAlerts(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	// Create a systemUp
	systemUp, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":   "test-system",
		"users":  []string{user.Id},
		"host":   "127.0.0.1",
		"status": "up",
	})
	assert.NoError(t, err)

	systemDown, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":   "test-system-2",
		"users":  []string{user.Id},
		"host":   "127.0.0.2",
		"status": "up",
	})
	assert.NoError(t, err)

	// Create a status alertUp for the system
	alertUp, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Status",
		"system": systemUp.Id,
		"user":   user.Id,
		"min":    1,
	})
	assert.NoError(t, err)

	alertDown, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "Status",
		"system": systemDown.Id,
		"user":   user.Id,
		"min":    1,
	})
	assert.NoError(t, err)

	// Verify alert is not triggered initially
	assert.False(t, alertUp.GetBool("triggered"), "Alert should not be triggered initially")

	// Set the system to 'up' (this should not trigger the alert)
	systemUp.Set("status", "up")
	err = hub.SaveNoValidate(systemUp)
	assert.NoError(t, err)

	systemDown.Set("status", "down")
	err = hub.SaveNoValidate(systemDown)
	assert.NoError(t, err)

	// Wait a moment for any processing
	time.Sleep(10 * time.Millisecond)

	// Verify alertUp is still not triggered after setting system to up
	alertUp, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alertUp.Id})
	assert.NoError(t, err)
	assert.False(t, alertUp.GetBool("triggered"), "Alert should not be triggered when system is up")

	// Manually set both alerts triggered to true
	alertUp.Set("triggered", true)
	err = hub.SaveNoValidate(alertUp)
	assert.NoError(t, err)
	alertDown.Set("triggered", true)
	err = hub.SaveNoValidate(alertDown)
	assert.NoError(t, err)

	// Verify we have exactly one alert with triggered true
	triggeredCount, err := hub.CountRecords("alerts", dbx.HashExp{"triggered": true})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, triggeredCount, "Should have exactly two alerts with triggered true")

	// Verify the specific alertUp is triggered
	alertUp, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alertUp.Id})
	assert.NoError(t, err)
	assert.True(t, alertUp.GetBool("triggered"), "Alert should be triggered")

	// Verify we have two unresolved alert history records
	alertHistoryCount, err := hub.CountRecords("alerts_history", dbx.HashExp{"resolved": ""})
	assert.NoError(t, err)
	assert.EqualValues(t, 2, alertHistoryCount, "Should have exactly two unresolved alert history records")

	err = alerts.ResolveStatusAlerts(hub)
	assert.NoError(t, err)

	// Verify alertUp is not triggered after resolving
	alertUp, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alertUp.Id})
	assert.NoError(t, err)
	assert.False(t, alertUp.GetBool("triggered"), "Alert should not be triggered after resolving")
	// Verify alertDown is still triggered
	alertDown, err = hub.FindFirstRecordByFilter("alerts", "id={:id}", dbx.Params{"id": alertDown.Id})
	assert.NoError(t, err)
	assert.True(t, alertDown.GetBool("triggered"), "Alert should still be triggered after resolving")

	// Verify we have one unresolved alert history record
	alertHistoryCount, err = hub.CountRecords("alerts_history", dbx.HashExp{"resolved": ""})
	assert.NoError(t, err)
	assert.EqualValues(t, 1, alertHistoryCount, "Should have exactly one unresolved alert history record")

}
