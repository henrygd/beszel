//go:build testing

package alerts_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

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
		"users": []string{regularUser.Id, adminUser.Id},
		"host":  "127.0.0.1",
	})

	system2, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system2",
		"users": []string{regularUser.Id, adminUser.Id},
		"host":  "127.0.0.2",
	})

	// system the admin is not a member of
	otherSystem, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system3",
		"users": []string{regularUser.Id},
		"host":  "127.0.0.3",
	})

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "POST no auth",
			Method:          http.MethodPost,
			URL:             "/api/beszel/alerts",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST regular user is forbidden",
			Method: http.MethodPost,
			URL:    "/api/beszel/alerts",
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
			URL:    "/api/beszel/alerts",
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
			URL:    "/api/beszel/alerts",
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
			URL:    "/api/beszel/alerts",
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
			URL:    "/api/beszel/alerts",
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
			URL:    "/api/beszel/alerts",
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
			Name:   "POST ignores systems the admin cannot access",
			Method: http.MethodPost,
			URL:    "/api/beszel/alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "Disk",
				"value":   85,
				"min":     5,
				"systems": []string{system1.Id, otherSystem.Id},
			}),
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				created, _ := app.CountRecords("alerts", dbx.HashExp{"name": "Disk", "system": system1.Id})
				assert.EqualValues(t, 1, created, "should create alert on accessible system")
				skipped, _ := app.CountRecords("alerts", dbx.HashExp{"name": "Disk", "system": otherSystem.Id})
				assert.Zero(t, skipped, "should skip system the admin cannot access")
			},
		},
		{
			Name:   "POST overwrite:false should not overwrite existing alert",
			Method: http.MethodPost,
			URL:    "/api/beszel/alerts",
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
			URL:    "/api/beszel/alerts",
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
			URL:             "/api/beszel/alerts",
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
			URL:    "/api/beszel/alerts",
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
			URL:    "/api/beszel/alerts",
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
			Name:   "DELETE ignores systems the admin cannot access",
			Method: http.MethodDelete,
			URL:    "/api/beszel/alerts",
			Headers: map[string]string{
				"Authorization": adminUserToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"count\":0", "\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"systems": []string{otherSystem.Id},
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.ClearCollection(t, app, "alerts")
				beszelTests.CreateRecord(app, "alerts", map[string]any{
					"name":   "CPU",
					"system": otherSystem.Id,
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
			Name:   "DELETE admin deletes alert across multiple systems",
			Method: http.MethodDelete,
			URL:    "/api/beszel/alerts",
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

	var delivered atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
	}))
	defer server.Close()
	localURL := "generic+" + server.URL

	readonlyUser, err := beszelTests.CreateUserWithRole(hub, "readonly@example.com", "password123", "readonly")
	assert.NoError(t, err)
	readonlyToken, err := readonlyUser.NewAuthToken()
	assert.NoError(t, err)
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
				"url": localURL,
			}),
		},
		{
			Name:           "POST /test-notification - invalid service reports error",
			Method:         http.MethodPost,
			URL:            "/api/beszel/test-notification",
			TestAppFactory: testAppFactory,
			Headers: map[string]string{
				"Authorization": userToken,
			},
			Body: jsonReader(map[string]any{
				"url": "unknown://example.com",
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
				"url": localURL,
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"err\":false"},
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
				"url": localURL,
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"err\":"},
		},
	}

	for _, url := range []string{localURL, "smtp://user:pass@127.0.0.1/?fromAddress=sender@example.com&toAddresses=recipient@example.com", "mqtt://127.0.0.1/topic"} {
		scenarios = append(scenarios, beszelTests.ApiScenario{
			BeforeTestFunc: func(tb testing.TB, _ *pbTests.TestApp, e *core.ServeEvent) {
				if !strings.HasPrefix(url, "mqtt://") {
					return
				}
				// Keep the real MQTT rejection path, but advance its library's
				// fixed timeout using virtual time instead of waiting 10 seconds.
				e.Router.BindFunc(func(re *core.RequestEvent) error {
					var err error
					synctest.Test(tb.(*testing.T), func(t *testing.T) {
						err = re.Next()
					})
					return err
				})
			},
			Name:            "readonly cannot send to " + url,
			Method:          http.MethodPost,
			URL:             "/api/beszel/test-notification",
			TestAppFactory:  testAppFactory,
			Headers:         map[string]string{"Authorization": readonlyToken},
			Body:            jsonReader(map[string]any{"url": url}),
			ExpectedStatus:  403,
			ExpectedContent: []string{"Only admins"},
		})
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
	assert.EqualValues(t, 2, delivered.Load(), "only admin and superuser requests should reach the server")
}
