//go:build testing

package monitors_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/hub/monitors"
	_ "github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

// sharedAPIApp builds one TestApp with a user + token + monitor routes,
// reused across scenarios via TestAppFactory (repo pattern).
func sharedAPIApp(t *testing.T) (*pbtests.TestApp, string) {
	t.Helper()
	app, err := pbtests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "api@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))
	token, err := user.NewAuthToken()
	require.NoError(t, err)

	monitors.RegisterRoutes(app)
	return app, token
}

func TestMonitorAPI_CreateValidation(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }

	scenarios := []pbtests.ApiScenario{
		{
			Name:   "create monitor with defaults",
			Method: http.MethodPost,
			URL:    "/api/beszel/monitors",
			Body:   strings.NewReader(`{"name":"web","type":"http","target":"https://example.com"}`),
			Headers: map[string]string{
				"Authorization": token,
				"Content-Type":  "application/json",
			},
			ExpectedStatus:        200,
			ExpectedContent:       []string{`"name":"web"`, `"status":"pending"`, `"interval":60`, `"timeout":10`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
		{
			Name:   "reject timeout >= interval",
			Method: http.MethodPost,
			URL:    "/api/beszel/monitors",
			Body:   strings.NewReader(`{"name":"bad","type":"http","target":"https://example.com","interval":60,"timeout":60}`),
			Headers: map[string]string{
				"Authorization": token,
				"Content-Type":  "application/json",
			},
			ExpectedStatus:        400,
			ExpectedContent:       []string{"Timeout"},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
		{
			Name:   "reject unknown type",
			Method: http.MethodPost,
			URL:    "/api/beszel/monitors",
			Body:   strings.NewReader(`{"name":"bad","type":"gopher","target":"example.com"}`),
			Headers: map[string]string{
				"Authorization": token,
				"Content-Type":  "application/json",
			},
			ExpectedStatus:        400,
			ExpectedContent:       []string{"Unknown monitor type"},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
		{
			Name:                  "unauthenticated rejected",
			Method:                http.MethodGet,
			URL:                   "/api/beszel/monitors",
			ExpectedStatus:        401,
			ExpectedContent:       []string{"requires valid"},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestMonitorAPI_SecretsRedactedAndKept(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token, "Content-Type": "application/json"}

	create := pbtests.ApiScenario{
		Name:   "create with bearer secret",
		Method: http.MethodPost,
		URL:    "/api/beszel/monitors",
		Body: strings.NewReader(`{"name":"secret","type":"http","target":"https://example.com",` +
			`"config":{"auth_type":"bearer","token":"s3cr3t-live"}}`),
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`••••••`},
		NotExpectedContent:    []string{"s3cr3t-live"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	create.Test(t)

	list := pbtests.ApiScenario{
		Name:                  "list redacts secrets",
		Method:                http.MethodGet,
		URL:                   "/api/beszel/monitors",
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`••••••`},
		NotExpectedContent:    []string{"s3cr3t-live"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	list.Test(t)
}

func TestMonitorAPI_TestEndpointRateLimited(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token, "Content-Type": "application/json"}

	create := pbtests.ApiScenario{
		Name:                  "create ping monitor",
		Method:                http.MethodPost,
		URL:                   "/api/beszel/monitors",
		Body:                  strings.NewReader(`{"name":"p","type":"ping","target":"192.0.2.1","interval":60,"timeout":10,"config":{"count":1,"packet_timeout":"1s"}}`),
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`"name":"p"`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	create.Test(t)

	var id string
	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	require.Len(t, recs, 1)
	id = recs[0].Id

	first := pbtests.ApiScenario{
		Name:                  "first test runs",
		Method:                http.MethodPost,
		URL:                   "/api/beszel/monitors/" + id + "/test",
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`"status":`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	first.Test(t)

	second := pbtests.ApiScenario{
		Name:                  "second test rate limited",
		Method:                http.MethodPost,
		URL:                   "/api/beszel/monitors/" + id + "/test",
		Headers:               auth,
		ExpectedStatus:        429,
		ExpectedContent:       []string{"rate limited"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	second.Test(t)

	total, err := app.CountRecords("monitor_checks")
	require.NoError(t, err)
	require.EqualValues(t, 0, total, "manual tests must not write history")
}

func TestMonitorAPI_SummaryAndAccess(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token, "Content-Type": "application/json"}

	create := pbtests.ApiScenario{
		Name:                  "create monitor",
		Method:                http.MethodPost,
		URL:                   "/api/beszel/monitors",
		Body:                  strings.NewReader(`{"name":"s","type":"dns","target":"example.com"}`),
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`"name":"s"`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	create.Test(t)

	summary := pbtests.ApiScenario{
		Name:                  "summary counts",
		Method:                http.MethodGet,
		URL:                   "/api/beszel/monitors/summary",
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`"pending":1`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	summary.Test(t)

	// Second user must not see the first user's monitor.
	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	other := core.NewRecord(users)
	other.Set("email", "other@example.com")
	other.Set("password", "password12345")
	require.NoError(t, app.Save(other))
	otherToken, err := other.NewAuthToken()
	require.NoError(t, err)

	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	require.Len(t, recs, 1)

	denied := pbtests.ApiScenario{
		Name:   "non-member cannot view",
		Method: http.MethodGet,
		URL:    "/api/beszel/monitors/" + recs[0].Id,
		Headers: map[string]string{
			"Authorization": otherToken,
		},
		ExpectedStatus:        404,
		ExpectedContent:       []string{"Monitor not found"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	denied.Test(t)

	empty := pbtests.ApiScenario{
		Name:   "empty history",
		Method: http.MethodGet,
		URL:    "/api/beszel/monitors/" + recs[0].Id + "/checks",
		Headers: map[string]string{
			"Authorization": token,
		},
		ExpectedStatus:        200,
		ExpectedContent:       []string{`[]`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	empty.Test(t)
}

func TestMonitorAPI_UpdateAndDelete(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token, "Content-Type": "application/json"}

	create := pbtests.ApiScenario{
		Name:   "create with secret",
		Method: http.MethodPost,
		URL:    "/api/beszel/monitors",
		Body: strings.NewReader(`{"name":"u","type":"http","target":"https://example.com",` +
			`"config":{"auth_type":"basic","username":"u","password":"pw-live"}}`),
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`••••••`},
		NotExpectedContent:    []string{"pw-live"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	create.Test(t)

	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	require.Len(t, recs, 1)
	id := recs[0].Id

	patch := pbtests.ApiScenario{
		Name:                  "patch keeps secret when redacted",
		Method:                http.MethodPatch,
		URL:                   "/api/beszel/monitors/" + id,
		Body:                  strings.NewReader(`{"name":"u2","config":{"auth_type":"basic","username":"u","password":"••••••"}}`),
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`"name":"u2"`, `••••••`},
		NotExpectedContent:    []string{"pw-live"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	patch.Test(t)

	stored, err := app.FindRecordById("monitors", id)
	require.NoError(t, err)
	cfg := map[string]any{}
	require.NoError(t, stored.UnmarshalJSONField("config", &cfg))
	require.Equal(t, "pw-live", cfg["password"], "redacted password must keep stored secret")

	badPatch := pbtests.ApiScenario{
		Name:                  "patch rejects timeout >= interval",
		Method:                http.MethodPatch,
		URL:                   "/api/beszel/monitors/" + id,
		Body:                  strings.NewReader(`{"timeout":3600,"interval":60}`),
		Headers:               auth,
		ExpectedStatus:        400,
		ExpectedContent:       []string{"Timeout"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	badPatch.Test(t)

	del := pbtests.ApiScenario{
		Name:                  "delete monitor",
		Method:                http.MethodDelete,
		URL:                   "/api/beszel/monitors/" + id,
		Headers:               auth,
		ExpectedStatus:        204,
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	del.Test(t)

	total, err := app.CountRecords("monitors")
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
}

func TestMonitorAPI_PatchTargetResetsFailures(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token, "Content-Type": "application/json"}

	create := pbtests.ApiScenario{
		Name:                  "create monitor",
		Method:                http.MethodPost,
		URL:                   "/api/beszel/monitors",
		Body:                  strings.NewReader(`{"name":"r","type":"http","target":"https://example.com"}`),
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`"name":"r"`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	create.Test(t)

	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	require.Len(t, recs, 1)
	id := recs[0].Id

	recs[0].Set("consecutive_failures", 2)
	require.NoError(t, app.SaveNoValidate(recs[0]))

	sameTarget := pbtests.ApiScenario{
		Name:                  "patch name only keeps failures",
		Method:                http.MethodPatch,
		URL:                   "/api/beszel/monitors/" + id,
		Body:                  strings.NewReader(`{"name":"r2"}`),
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`"name":"r2"`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	sameTarget.Test(t)
	stored, err := app.FindRecordById("monitors", id)
	require.NoError(t, err)
	require.EqualValues(t, 2, stored.GetFloat("consecutive_failures"))

	newTarget := pbtests.ApiScenario{
		Name:                  "patch target resets failures",
		Method:                http.MethodPatch,
		URL:                   "/api/beszel/monitors/" + id,
		Body:                  strings.NewReader(`{"target":"https://example.org"}`),
		Headers:               auth,
		ExpectedStatus:        200,
		ExpectedContent:       []string{`example.org`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
	}
	newTarget.Test(t)
	stored, err = app.FindRecordById("monitors", id)
	require.NoError(t, err)
	require.EqualValues(t, 0, stored.GetFloat("consecutive_failures"))
}
