//go:build testing

package alerts_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	pbTests "github.com/pocketbase/pocketbase/tests"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

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

func TestIsInternalURL(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		internal bool
	}{
		{name: "loopback ipv4", url: "generic://127.0.0.1", internal: true},
		{name: "localhost hostname", url: "generic://localhost", internal: true},
		{name: "localhost hostname", url: "generic+http://localhost/api/v1/postStuff", internal: true},
		{name: "localhost hostname", url: "generic+http://127.0.0.1:8080/api/v1/postStuff", internal: true},
		{name: "localhost hostname", url: "generic+https://beszel.dev/api/v1/postStuff", internal: false},
		{name: "public ipv4", url: "generic://8.8.8.8", internal: false},
		{name: "token style service url", url: "discord://abc123@123456789", internal: false},
		{name: "single label service url", url: "slack://token@team/channel", internal: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			internal, err := alerts.IsInternalURL(testCase.url)
			assert.NoError(t, err)
			assert.Equal(t, testCase.internal, internal)
		})
	}
}

func TestAlertsApi(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	hub.StartHub()

	// Regular user (cannot manage alerts)
	regularUser, _ := beszelTests.CreateUser(hub, "user@example.com", "password")
	regularUserToken, _ := regularUser.NewAuthToken()

	// Admin user (can manage alerts)
	adminUser, _ := beszelTests.CreateUserWithRole(hub, "admin@example.com", "password", "admin")
	adminUserToken, _ := adminUser.NewAuthToken()

	system1, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system1",
		"users": []string{regularUser.Id},
		"host":  "127.0.0.1",
	})

	system2, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system2",
		"users": []string{regularUser.Id},
		"host":  "127.0.0.2",
	})

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "POST no auth",
			Method:          http.MethodPost,
			URL:             "/api/beszel/user-alerts",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST regular user is forbidden",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": regularUserToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"Only admins"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"value":   80,
				"min":     5,
				"systems": []string{system1.Id},
			}),
		},
		{
			Name:   "POST no body",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
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
				"Authorization": adminUserToken,
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
				"Authorization": adminUserToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Bad data"},
			TestAppFactory:  testAppFactory,
			Body:            strings.NewReader(`{"alertType": "cpu", "threshold": 80, "enabled": true,}`),
		},
		{
			Name:   "POST admin creates alert for multiple systems",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
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
				alertCount, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 2, alertCount, "should have 2 alerts")
				matchingAlerts, _ := app.CountRecords("alerts", dbx.HashExp{"name": "CPU", "system": system1.Id, "value": 69, "min": 9})
				assert.EqualValues(t, 1, matchingAlerts, "should have 1 alert for system1")
			},
		},
		{
			Name:   "POST admin creates alert for single system",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
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
				alertCount, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 3, alertCount, "should have 3 alerts total")
			},
		},
		{
			Name:   "POST overwrite:false should not overwrite existing alert",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
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
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alertCount, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 1, alertCount, "should have 1 alert")
				alert, _ := app.FindFirstRecordByFilter("alerts", "name = 'CPU' && system = {:system}", dbx.Params{"system": system1.Id})
				assert.EqualValues(t, 80, alert.Get("value"), "should have 80 as value (not overwritten)")
			},
		},
		{
			Name:   "POST overwrite:true should overwrite existing alert",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
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
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alertCount, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 1, alertCount, "should have 1 alert")
				alert, _ := app.FindFirstRecordByFilter("alerts", "name = 'CPU' && system = {:system}", dbx.Params{"system": system2.Id})
				assert.EqualValues(t, 45, alert.Get("value"), "should have 45 as value (overwritten)")
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
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alertCount, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 1, alertCount, "should have 1 alert (not deleted)")
			},
		},
		{
			Name:   "DELETE regular user is forbidden",
			Method: http.MethodDelete,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": regularUserToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"Only admins"},
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
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alertCount, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 1, alertCount, "should have 1 alert (not deleted)")
			},
		},
		{
			Name:   "DELETE admin deletes alert",
			Method: http.MethodDelete,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
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
					"value":  80,
					"min":    10,
				})
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alertCount, _ := app.CountRecords("alerts")
				assert.Zero(t, alertCount, "should have 0 alerts")
			},
		},
		{
			Name:   "DELETE admin deletes alert across multiple systems",
			Method: http.MethodDelete,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
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
						"value":  90,
						"min":    10,
					})
					assert.NoError(t, err, "should create alert")
				}
				alertCount, _ := app.CountRecords("alerts")
				assert.EqualValues(t, 2, alertCount, "should have 2 alerts before delete")
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				alertCount, _ := app.CountRecords("alerts")
				assert.Zero(t, alertCount, "should have 0 alerts")
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestSendTestNotification(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	userToken, err := user.NewAuthToken()

	adminUser, err := beszelTests.CreateUserWithRole(hub, "admin@example.com", "password123", "admin")
	assert.NoError(t, err, "Failed to create admin user")
	adminUserToken, err := adminUser.NewAuthToken()

	superuser, err := beszelTests.CreateSuperuser(hub, "superuser@example.com", "password123")
	assert.NoError(t, err, "Failed to create superuser")
	superuserToken, err := superuser.NewAuthToken()
	assert.NoError(t, err, "Failed to create superuser auth token")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "POST /test-notification - no auth should fail",
			Method:          http.MethodPost,
			URL:             "/api/beszel/test-notification",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"url": "generic://127.0.0.1",
			}),
		},
		{
			Name:           "POST /test-notification - with external auth should succeed",
			Method:         http.MethodPost,
			URL:            "/api/beszel/test-notification",
			TestAppFactory: testAppFactory,
			Headers: map[string]string{
				"Authorization": userToken,
			},
			Body: jsonReader(map[string]any{
				"url": "generic://8.8.8.8",
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"err\":"},
		},
		{
			Name:           "POST /test-notification - local url with user auth should fail",
			Method:         http.MethodPost,
			URL:            "/api/beszel/test-notification",
			TestAppFactory: testAppFactory,
			Headers: map[string]string{
				"Authorization": userToken,
			},
			Body: jsonReader(map[string]any{
				"url": "generic://localhost:8010",
			}),
			ExpectedStatus:  403,
			ExpectedContent: []string{"Only admins"},
		},
		{
			Name:           "POST /test-notification - internal url with user auth should fail",
			Method:         http.MethodPost,
			URL:            "/api/beszel/test-notification",
			TestAppFactory: testAppFactory,
			Headers: map[string]string{
				"Authorization": userToken,
			},
			Body: jsonReader(map[string]any{
				"url": "generic+http://192.168.0.5",
			}),
			ExpectedStatus:  403,
			ExpectedContent: []string{"Only admins"},
		},
		{
			Name:           "POST /test-notification - internal url with admin auth should succeed",
			Method:         http.MethodPost,
			URL:            "/api/beszel/test-notification",
			TestAppFactory: testAppFactory,
			Headers: map[string]string{
				"Authorization": adminUserToken,
			},
			Body: jsonReader(map[string]any{
				"url": "generic://127.0.0.1",
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"err\":"},
		},
		{
			Name:           "POST /test-notification - internal url with superuser auth should succeed",
			Method:         http.MethodPost,
			URL:            "/api/beszel/test-notification",
			TestAppFactory: testAppFactory,
			Headers: map[string]string{
				"Authorization": superuserToken,
			},
			Body: jsonReader(map[string]any{
				"url": "generic://127.0.0.1",
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"err\":"},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
