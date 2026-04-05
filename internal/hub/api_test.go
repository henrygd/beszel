package hub_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/henrygd/beszel/internal/migrations"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

// marshal to json and return an io.Reader (for use in ApiScenario.Body)
func jsonReader(v any) io.Reader {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(data)
}

func TestApiRoutesAuthentication(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()

	userToken, err := user.NewAuthToken()
	require.NoError(t, err, "Failed to create auth token")

	// Create test user and get auth token
	user2, err := beszelTests.CreateUser(hub, "testuser@example.com", "password123")
	require.NoError(t, err, "Failed to create test user")
	user2Token, err := user2.NewAuthToken()
	require.NoError(t, err, "Failed to create user2 auth token")

	adminUser, err := beszelTests.CreateUserWithRole(hub, "admin@example.com", "password123", "admin")
	require.NoError(t, err, "Failed to create admin user")
	adminUserToken, err := adminUser.NewAuthToken()

	readOnlyUser, err := beszelTests.CreateUserWithRole(hub, "readonly@example.com", "password123", "readonly")
	require.NoError(t, err, "Failed to create readonly user")
	readOnlyUserToken, err := readOnlyUser.NewAuthToken()
	require.NoError(t, err, "Failed to create readonly user auth token")

	superuser, err := beszelTests.CreateSuperuser(hub, "superuser@example.com", "password123")
	require.NoError(t, err, "Failed to create superuser")
	superuserToken, err := superuser.NewAuthToken()
	require.NoError(t, err, "Failed to create superuser auth token")

	// Create test system
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	require.NoError(t, err, "Failed to create test system")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		// Auth Protected Routes - Should require authentication
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
			Name:           "POST /test-notification - with auth should succeed",
			Method:         http.MethodPost,
			URL:            "/api/beszel/test-notification",
			TestAppFactory: testAppFactory,
			Headers: map[string]string{
				"Authorization": userToken,
			},
			Body: jsonReader(map[string]any{
				"url": "generic://127.0.0.1",
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"sending message"},
		},
		{
			Name:            "GET /config-yaml - no auth should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/config-yaml",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /config-yaml - with user auth should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/config-yaml",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"The authorized record is not allowed to perform this action."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /config-yaml - with admin auth should succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/config-yaml",
			Headers: map[string]string{
				"Authorization": adminUserToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"test-system"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /heartbeat-status - no auth should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/heartbeat-status",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /heartbeat-status - with user auth should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/heartbeat-status",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"The authorized record is not allowed to perform this action."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /heartbeat-status - with admin auth should succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/heartbeat-status",
			Headers: map[string]string{
				"Authorization": adminUserToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"enabled":false`},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /test-heartbeat - with user auth should fail",
			Method: http.MethodPost,
			URL:    "/api/beszel/test-heartbeat",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"The authorized record is not allowed to perform this action."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /test-heartbeat - with admin auth should report disabled state",
			Method: http.MethodPost,
			URL:    "/api/beszel/test-heartbeat",
			Headers: map[string]string{
				"Authorization": adminUserToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"Heartbeat not configured"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /universal-token - no auth should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/universal-token",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /universal-token - with auth should succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/universal-token",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"active", "token", "permanent"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /universal-token - enable permanent should succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/universal-token?enable=1&permanent=1&token=permanent-token-123",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"permanent\":true", "permanent-token-123"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /universal-token - superuser should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/universal-token",
			Headers: map[string]string{
				"Authorization": superuserToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"Superusers cannot use universal tokens"},
			TestAppFactory: func(t testing.TB) *pbTests.TestApp {
				return hub.TestApp
			},
		},
		{
			Name:   "GET /universal-token - with readonly auth should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/universal-token",
			Headers: map[string]string{
				"Authorization": readOnlyUserToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"The authorized record is not allowed to perform this action."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /smart/refresh - missing system should fail 400 with user auth",
			Method: http.MethodPost,
			URL:    "/api/beszel/smart/refresh",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid", "system", "parameter"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /smart/refresh - with readonly auth should fail",
			Method: http.MethodPost,
			URL:    fmt.Sprintf("/api/beszel/smart/refresh?system=%s", system.Id),
			Headers: map[string]string{
				"Authorization": readOnlyUserToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"The authorized record is not allowed to perform this action."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /smart/refresh - non-user system should fail",
			Method: http.MethodPost,
			URL:    fmt.Sprintf("/api/beszel/smart/refresh?system=%s", system.Id),
			Headers: map[string]string{
				"Authorization": user2Token,
			},
			ExpectedStatus:  404,
			ExpectedContent: []string{"The requested resource wasn't found."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /smart/refresh - good user should pass validation",
			Method: http.MethodPost,
			URL:    fmt.Sprintf("/api/beszel/smart/refresh?system=%s", system.Id),
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  500,
			ExpectedContent: []string{"Something went wrong while processing your request."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "POST /user-alerts - no auth should fail",
			Method:          http.MethodPost,
			URL:             "/api/beszel/user-alerts",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"value":   80,
				"min":     10,
				"systems": []string{system.Id},
			}),
		},
		{
			Name:   "POST /user-alerts - with auth should succeed",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"value":   80,
				"min":     10,
				"systems": []string{system.Id},
			}),
		},
		{
			Name:            "DELETE /user-alerts - no auth should fail",
			Method:          http.MethodDelete,
			URL:             "/api/beszel/user-alerts",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"systems": []string{system.Id},
			}),
		},
		{
			Name:   "DELETE /user-alerts - with auth should succeed",
			Method: http.MethodDelete,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"systems": []string{system.Id},
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				// Create an alert to delete
				beszelTests.CreateRecord(app, "alerts", map[string]any{
					"name":   "CPU",
					"system": system.Id,
					"user":   user.Id,
					"value":  80,
					"min":    10,
				})
			},
		},
		{
			Name:            "GET /containers/logs - no auth should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/containers/logs?system=test-system&container=abababababab",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /containers/logs - request for valid non-user system should fail",
			Method:          http.MethodGet,
			URL:             fmt.Sprintf("/api/beszel/containers/logs?system=%s&container=abababababab", system.Id),
			ExpectedStatus:  404,
			ExpectedContent: []string{"The requested resource wasn't found."},
			TestAppFactory:  testAppFactory,
			Headers: map[string]string{
				"Authorization": user2Token,
			},
		},
		{
			Name:            "GET /containers/info - request for valid non-user system should fail",
			Method:          http.MethodGet,
			URL:             fmt.Sprintf("/api/beszel/containers/info?system=%s&container=abababababab", system.Id),
			ExpectedStatus:  404,
			ExpectedContent: []string{"The requested resource wasn't found."},
			TestAppFactory:  testAppFactory,
			Headers: map[string]string{
				"Authorization": user2Token,
			},
		},
		{
			Name:   "GET /containers/logs - with auth but missing system param should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/containers/logs?container=abababababab",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid", "parameter"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /containers/logs - with auth but missing container param should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/containers/logs?system=test-system",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid", "parameter"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /containers/logs - with auth but invalid system should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/containers/logs?system=invalid-system&container=0123456789ab",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  404,
			ExpectedContent: []string{"The requested resource wasn't found."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /containers/logs - traversal container should fail validation",
			Method: http.MethodGet,
			URL:    "/api/beszel/containers/logs?system=" + system.Id + "&container=..%2F..%2Fversion",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid", "parameter"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /containers/info - traversal container should fail validation",
			Method: http.MethodGet,
			URL:    "/api/beszel/containers/info?system=" + system.Id + "&container=../../version?x=",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid", "parameter"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /containers/info - non-hex container should fail validation",
			Method: http.MethodGet,
			URL:    "/api/beszel/containers/info?system=" + system.Id + "&container=container_name",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid", "parameter"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /containers/logs - good user should pass validation",
			Method: http.MethodGet,
			URL:    "/api/beszel/containers/logs?system=" + system.Id + "&container=0123456789ab",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  500,
			ExpectedContent: []string{"Something went wrong while processing your request."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /containers/info - good user should pass validation",
			Method: http.MethodGet,
			URL:    "/api/beszel/containers/info?system=" + system.Id + "&container=0123456789ab",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  500,
			ExpectedContent: []string{"Something went wrong while processing your request."},
			TestAppFactory:  testAppFactory,
		},
		// /systemd routes
		{
			Name:            "GET /systemd/info - no auth should fail",
			Method:          http.MethodGet,
			URL:             fmt.Sprintf("/api/beszel/systemd/info?system=%s&service=nginx.service", system.Id),
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /systemd/info - request for valid non-user system should fail",
			Method:          http.MethodGet,
			URL:             fmt.Sprintf("/api/beszel/systemd/info?system=%s&service=nginx.service", system.Id),
			ExpectedStatus:  404,
			ExpectedContent: []string{"The requested resource wasn't found."},
			TestAppFactory:  testAppFactory,
			Headers: map[string]string{
				"Authorization": user2Token,
			},
		},
		{
			Name:   "GET /systemd/info - with auth but missing system param should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/systemd/info?service=nginx.service",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid", "parameter"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /systemd/info - with auth but missing service param should fail",
			Method: http.MethodGet,
			URL:    fmt.Sprintf("/api/beszel/systemd/info?system=%s", system.Id),
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid", "parameter"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /systemd/info - with auth but invalid system should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/systemd/info?system=invalid-system&service=nginx.service",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  404,
			ExpectedContent: []string{"The requested resource wasn't found."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /systemd/info - service not in systemd_services collection should fail",
			Method: http.MethodGet,
			URL:    fmt.Sprintf("/api/beszel/systemd/info?system=%s&service=notregistered.service", system.Id),
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  404,
			ExpectedContent: []string{"The requested resource wasn't found."},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /systemd/info - with auth and existing service record should pass validation",
			Method: http.MethodGet,
			URL:    fmt.Sprintf("/api/beszel/systemd/info?system=%s&service=nginx.service", system.Id),
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  500,
			ExpectedContent: []string{"Something went wrong while processing your request."},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.CreateRecord(app, "systemd_services", map[string]any{
					"system": system.Id,
					"name":   "nginx.service",
					"state":  0,
					"sub":    1,
				})
			},
		},

		// Auth Optional Routes - Should work without authentication
		{
			Name:            "GET /getkey - no auth should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /getkey - with auth should also succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/getkey",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"key\":", "\"v\":"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /info - should return the same as /getkey",
			Method: http.MethodGet,
			URL:    "/api/beszel/info",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"key\":", "\"v\":"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /first-run - no auth should succeed",
			Method:          http.MethodGet,
			URL:             "/api/beszel/first-run",
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"firstRun\":false"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /first-run - with auth should also succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/first-run",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"firstRun\":false"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /agent-connect - no auth should succeed (websocket upgrade fails but route is accessible)",
			Method:          http.MethodGet,
			URL:             "/api/beszel/agent-connect",
			ExpectedStatus:  400,
			ExpectedContent: []string{},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /test-notification - invalid auth token should fail",
			Method: http.MethodPost,
			URL:    "/api/beszel/test-notification",
			Body: jsonReader(map[string]any{
				"url": "generic://127.0.0.1",
			}),
			Headers: map[string]string{
				"Authorization": "invalid-token",
			},
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /user-alerts - invalid auth token should fail",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": "invalid-token",
			},
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"value":   80,
				"min":     10,
				"systems": []string{system.Id},
			}),
		},
		// this works but diff behavior on prod vs dev.
		// dev returns 502; prod returns 200 with static html page 404
		// TODO: align dev and prod behavior and re-enable this test
		// {
		// 	Name:               "GET /update - shouldn't exist without CHECK_UPDATES env var",
		// 	Method:             http.MethodGet,
		// 	URL:                "/api/beszel/update",
		// 	NotExpectedContent: []string{"v:", "\"v\":"},
		// 	ExpectedStatus: 502,
		// 	TestAppFactory: testAppFactory,
		// },
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestFirstUserCreation(t *testing.T) {
	t.Run("CreateUserEndpoint available when no users exist", func(t *testing.T) {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		defer hub.Cleanup()

		hub.StartHub()

		testAppFactoryExisting := func(t testing.TB) *pbTests.TestApp {
			return hub.TestApp
		}

		scenarios := []beszelTests.ApiScenario{
			{
				Name:   "POST /create-user - should be available when no users exist",
				Method: http.MethodPost,
				URL:    "/api/beszel/create-user",
				Body: jsonReader(map[string]any{
					"email":    "firstuser@example.com",
					"password": "password123",
				}),
				ExpectedStatus:  200,
				ExpectedContent: []string{"User created"},
				TestAppFactory:  testAppFactoryExisting,
				BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
					userCount, err := hub.CountRecords("users")
					require.NoError(t, err)
					require.Zero(t, userCount, "Should start with no users")
					superusers, err := hub.FindAllRecords(core.CollectionNameSuperusers)
					require.NoError(t, err)
					require.EqualValues(t, 1, len(superusers), "Should start with one temporary superuser")
					require.EqualValues(t, migrations.TempAdminEmail, superusers[0].GetString("email"), "Should have created one temporary superuser")
				},
				AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
					userCount, err := hub.CountRecords("users")
					require.NoError(t, err)
					require.EqualValues(t, 1, userCount, "Should have created one user")
					superusers, err := hub.FindAllRecords(core.CollectionNameSuperusers)
					require.NoError(t, err)
					require.EqualValues(t, 1, len(superusers), "Should have created one superuser")
					require.EqualValues(t, "firstuser@example.com", superusers[0].GetString("email"), "Should have created one superuser")
				},
			},
			{
				Name:   "POST /create-user - should not be available when users exist",
				Method: http.MethodPost,
				URL:    "/api/beszel/create-user",
				Body: jsonReader(map[string]any{
					"email":    "firstuser@example.com",
					"password": "password123",
				}),
				ExpectedStatus:  404,
				ExpectedContent: []string{"wasn't found"},
				TestAppFactory:  testAppFactoryExisting,
			},
		}

		for _, scenario := range scenarios {
			scenario.Test(t)
		}
	})

	t.Run("CreateUserEndpoint not available when USER_EMAIL, USER_PASSWORD are set", func(t *testing.T) {
		t.Setenv("BESZEL_HUB_USER_EMAIL", "me@example.com")
		t.Setenv("BESZEL_HUB_USER_PASSWORD", "password123")

		hub, _ := beszelTests.NewTestHub(t.TempDir())
		defer hub.Cleanup()

		hub.StartHub()

		testAppFactory := func(t testing.TB) *pbTests.TestApp {
			return hub.TestApp
		}

		scenario := beszelTests.ApiScenario{
			Name:            "POST /create-user - should not be available when USER_EMAIL, USER_PASSWORD are set",
			Method:          http.MethodPost,
			URL:             "/api/beszel/create-user",
			ExpectedStatus:  404,
			ExpectedContent: []string{"wasn't found"},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				users, err := hub.FindAllRecords("users")
				require.NoError(t, err)
				require.EqualValues(t, 1, len(users), "Should start with one user")
				require.EqualValues(t, "me@example.com", users[0].GetString("email"), "Should have created one user")
				superusers, err := hub.FindAllRecords(core.CollectionNameSuperusers)
				require.NoError(t, err)
				require.EqualValues(t, 1, len(superusers), "Should start with one superuser")
				require.EqualValues(t, "me@example.com", superusers[0].GetString("email"), "Should have created one superuser")
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				users, err := hub.FindAllRecords("users")
				require.NoError(t, err)
				require.EqualValues(t, 1, len(users), "Should still have one user")
				require.EqualValues(t, "me@example.com", users[0].GetString("email"), "Should have created one user")
				superusers, err := hub.FindAllRecords(core.CollectionNameSuperusers)
				require.NoError(t, err)
				require.EqualValues(t, 1, len(superusers), "Should still have one superuser")
				require.EqualValues(t, "me@example.com", superusers[0].GetString("email"), "Should have created one superuser")
			},
		}

		scenario.Test(t)
	})
}

func TestCreateUserEndpointAvailability(t *testing.T) {
	t.Run("CreateUserEndpoint available when no users exist", func(t *testing.T) {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		defer hub.Cleanup()

		// Ensure no users exist
		userCount, err := hub.CountRecords("users")
		require.NoError(t, err)
		require.Zero(t, userCount, "Should start with no users")

		hub.StartHub()

		testAppFactory := func(t testing.TB) *pbTests.TestApp {
			return hub.TestApp
		}

		scenario := beszelTests.ApiScenario{
			Name:   "POST /create-user - should be available when no users exist",
			Method: http.MethodPost,
			URL:    "/api/beszel/create-user",
			Body: jsonReader(map[string]any{
				"email":    "firstuser@example.com",
				"password": "password123",
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"User created"},
			TestAppFactory:  testAppFactory,
		}

		scenario.Test(t)

		// Verify user was created
		userCount, err = hub.CountRecords("users")
		require.NoError(t, err)
		require.EqualValues(t, 1, userCount, "Should have created one user")
	})

	t.Run("CreateUserEndpoint not available when users exist", func(t *testing.T) {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		defer hub.Cleanup()

		// Create a user first
		_, err := beszelTests.CreateUser(hub, "existing@example.com", "password")
		require.NoError(t, err)

		hub.StartHub()

		testAppFactory := func(t testing.TB) *pbTests.TestApp {
			return hub.TestApp
		}

		scenario := beszelTests.ApiScenario{
			Name:   "POST /create-user - should not be available when users exist",
			Method: http.MethodPost,
			URL:    "/api/beszel/create-user",
			Body: jsonReader(map[string]any{
				"email":    "another@example.com",
				"password": "password123",
			}),
			ExpectedStatus:  404,
			ExpectedContent: []string{"wasn't found"},
			TestAppFactory:  testAppFactory,
		}

		scenario.Test(t)
	})
}

func TestAutoLoginMiddleware(t *testing.T) {
	var hubs []*beszelTests.TestHub

	defer func() {
		for _, hub := range hubs {
			hub.Cleanup()
		}
	}()

	t.Setenv("AUTO_LOGIN", "user@test.com")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		hubs = append(hubs, hub)
		hub.StartHub()
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "GET /getkey - without auto login should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /getkey - with auto login should fail if no matching user",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /getkey - with auto login should succeed",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"key\":", "\"v\":"},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.CreateUser(app, "user@test.com", "password123")
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestTrustedHeaderMiddleware(t *testing.T) {
	var hubs []*beszelTests.TestHub

	defer func() {
		for _, hub := range hubs {
			hub.Cleanup()
		}
	}()

	t.Setenv("TRUSTED_AUTH_HEADER", "X-Beszel-Trusted")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		hubs = append(hubs, hub)
		hub.StartHub()
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "GET /getkey - without trusted header should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /getkey - with trusted header should fail if no matching user",
			Method: http.MethodGet,
			URL:    "/api/beszel/getkey",
			Headers: map[string]string{
				"X-Beszel-Trusted": "user@test.com",
			},
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /getkey - with trusted header should succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/getkey",
			Headers: map[string]string{
				"X-Beszel-Trusted": "user@test.com",
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"key\":", "\"v\":"},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.CreateUser(app, "user@test.com", "password123")
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestUpdateEndpoint(t *testing.T) {
	t.Setenv("CHECK_UPDATES", "true")

	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()
	hub.StartHub()

	// Create test user and get auth token
	// user, err := beszelTests.CreateUser(hub, "testuser@example.com", "password123")
	// require.NoError(t, err, "Failed to create test user")
	// userToken, err := user.NewAuthToken()

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "update endpoint shouldn't work without auth",
			Method:          http.MethodGet,
			URL:             "/api/beszel/update",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		// leave this out for now since it actually makes a request to github
		// {
		// 	Name:   "GET /update - with valid auth should succeed",
		// 	Method: http.MethodGet,
		// 	URL:    "/api/beszel/update",
		// 	Headers: map[string]string{
		// 		"Authorization": userToken,
		// 	},
		// 	ExpectedStatus:  200,
		// 	ExpectedContent: []string{`"v":`},
		// 	TestAppFactory:  testAppFactory,
		// },
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
