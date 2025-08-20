//go:build testing
// +build testing

package alerts_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	beszelTests "beszel/internal/tests"

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
		{
			Name:            "GET not implemented - returns index",
			Method:          http.MethodGet,
			URL:             "/api/beszel/user-alerts",
			ExpectedStatus:  200,
			ExpectedContent: []string{"<html ", "globalThis.BESZEL"},
			TestAppFactory:  testAppFactory,
		},
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
