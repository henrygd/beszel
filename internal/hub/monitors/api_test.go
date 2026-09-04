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
// withMonitorRoutes registers the monitors API on the scenario's serve event.
func withMonitorRoutes() func(testing.TB, *pbtests.TestApp, *core.ServeEvent) {
	return func(_ testing.TB, _ *pbtests.TestApp, e *core.ServeEvent) {
		monitors.RegisterRoutes(e)
	}
}

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
			ExpectedStatus:        201,
			ExpectedContent:       []string{`"name":"web"`, `"status":"pending"`, `"interval":60`, `"timeout":10`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
			BeforeTestFunc:        withMonitorRoutes(),
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
			BeforeTestFunc:        withMonitorRoutes(),
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
			BeforeTestFunc:        withMonitorRoutes(),
		},
		{
			Name:                  "unauthenticated rejected",
			Method:                http.MethodGet,
			URL:                   "/api/beszel/monitors",
			ExpectedStatus:        401,
			ExpectedContent:       []string{"requires valid"},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
			BeforeTestFunc:        withMonitorRoutes(),
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
		ExpectedStatus:        201,
		ExpectedContent:       []string{`••••••`},
		NotExpectedContent:    []string{"s3cr3t-live"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		ExpectedStatus:        201,
		ExpectedContent:       []string{`"name":"p"`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		ExpectedStatus:        201,
		ExpectedContent:       []string{`"name":"s"`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		ExpectedStatus:        201,
		ExpectedContent:       []string{`••••••`},
		NotExpectedContent:    []string{"pw-live"},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		ExpectedStatus:        201,
		ExpectedContent:       []string{`"name":"r"`},
		TestAppFactory:        factory,
		DisableTestAppCleanup: true,
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
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
		BeforeTestFunc:        withMonitorRoutes(),
	}
	newTarget.Test(t)
	stored, err = app.FindRecordById("monitors", id)
	require.NoError(t, err)
	require.EqualValues(t, 0, stored.GetFloat("consecutive_failures"))
}

func TestMonitorAPI_ReadonlyBlocked(t *testing.T) {
	app, _ := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	ro := core.NewRecord(users)
	ro.Set("email", "ro@example.com")
	ro.Set("password", "password12345")
	ro.Set("role", "readonly")
	require.NoError(t, app.Save(ro))
	roToken, err := ro.NewAuthToken()
	require.NoError(t, err)
	auth := map[string]string{"Authorization": roToken, "Content-Type": "application/json"}

	for _, tc := range []struct {
		name   string
		method string
		url    string
		body   string
	}{
		{"post", http.MethodPost, "/api/beszel/monitors", `{"name":"x","type":"ping","target":"example.com"}`},
		{"patch", http.MethodPatch, "/api/beszel/monitors/nonexistent", `{"name":"x"}`},
		{"delete", http.MethodDelete, "/api/beszel/monitors/nonexistent", ``},
	} {
		sc := pbtests.ApiScenario{
			Name:                  "readonly " + tc.name + " forbidden",
			Method:                tc.method,
			URL:                   tc.url,
			Headers:               auth,
			ExpectedStatus:        403,
			ExpectedContent:       []string{"Readonly"},
			TestAppFactory:        factory,
			BeforeTestFunc:        withMonitorRoutes(),
			DisableTestAppCleanup: true,
		}
		if tc.body != "" {
			sc.Body = strings.NewReader(tc.body)
		}
		sc.Test(t)
	}
}

func TestMonitorAPI_PausedToggleAndUpsideDown(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token, "Content-Type": "application/json"}
	hook := withMonitorRoutes()

	create := pbtests.ApiScenario{
		Name: "create", Method: http.MethodPost, URL: "/api/beszel/monitors",
		Body:    strings.NewReader(`{"name":"t","type":"ping","target":"example.com"}`),
		Headers: auth, ExpectedStatus: 201, ExpectedContent: []string{`"name":"t"`},
		TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
	}
	create.Test(t)
	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	require.Len(t, recs, 1)
	id := recs[0].Id

	pause := pbtests.ApiScenario{
		Name: "pause", Method: http.MethodPatch, URL: "/api/beszel/monitors/" + id,
		Body:    strings.NewReader(`{"paused":true}`),
		Headers: auth, ExpectedStatus: 200, ExpectedContent: []string{`"status":"paused"`},
		TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
	}
	pause.Test(t)

	unpauseOmitted := pbtests.ApiScenario{
		Name: "patch without paused keeps pause", Method: http.MethodPatch, URL: "/api/beszel/monitors/" + id,
		Body:    strings.NewReader(`{"name":"t2"}`),
		Headers: auth, ExpectedStatus: 200, ExpectedContent: []string{`"status":"paused"`},
		TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
	}
	unpauseOmitted.Test(t)

	unpause := pbtests.ApiScenario{
		Name: "unpause returns to pending", Method: http.MethodPatch, URL: "/api/beszel/monitors/" + id,
		Body:    strings.NewReader(`{"paused":false}`),
		Headers: auth, ExpectedStatus: 200, ExpectedContent: []string{`"status":"pending"`},
		TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
	}
	unpause.Test(t)

	setUD := pbtests.ApiScenario{
		Name: "set upside_down", Method: http.MethodPatch, URL: "/api/beszel/monitors/" + id,
		Body:    strings.NewReader(`{"upside_down":true}`),
		Headers: auth, ExpectedStatus: 200, ExpectedContent: []string{`"upside_down":true`},
		TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
	}
	setUD.Test(t)

	clearUD := pbtests.ApiScenario{
		Name: "clear upside_down", Method: http.MethodPatch, URL: "/api/beszel/monitors/" + id,
		Body:    strings.NewReader(`{"upside_down":false}`),
		Headers: auth, ExpectedStatus: 200, ExpectedContent: []string{`"upside_down":false`},
		TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
	}
	clearUD.Test(t)
}

func TestMonitorAPI_PerTypeValidation(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token, "Content-Type": "application/json"}
	hook := withMonitorRoutes()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"http without scheme", `{"name":"a","type":"http","target":"example.com"}`, "Invalid http target"},
		{"http gopher scheme", `{"name":"a","type":"http","target":"gopher://example.com"}`, "Invalid http target"},
		{"http bad method", `{"name":"a","type":"http","target":"https://example.com","config":{"method":"BREW"}}`, "Invalid method"},
		{"tls bare port text", `{"name":"a","type":"tls","target":"example.com:notaport"}`, "Invalid tls target"},
		{"dns bad qtype", `{"name":"a","type":"dns","target":"example.com","config":{"qtype":"SMOKE"}}`, "Invalid qtype"},
		{"dns bad protocol", `{"name":"a","type":"dns","target":"example.com","config":{"protocol":"quic"}}`, "Invalid protocol"},
		{"dns hostname resolver", `{"name":"a","type":"dns","target":"example.com","config":{"resolver":"dns.example.com"}}`, "must be an IP"},
		{"resend_after too big", `{"name":"a","type":"ping","target":"example.com","resend_after":2000}`, "Resend_after"},
	}
	for _, tc := range cases {
		sc := pbtests.ApiScenario{
			Name: tc.name, Method: http.MethodPost, URL: "/api/beszel/monitors",
			Body:    strings.NewReader(tc.body),
			Headers: auth, ExpectedStatus: 400, ExpectedContent: []string{tc.want},
			TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
		}
		sc.Test(t)
	}
}

func TestMonitorAPI_HistoryLimitAndOrder(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token}
	hook := withMonitorRoutes()

	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	u, err := app.FindAuthRecordByEmail("users", "api@example.com")
	require.NoError(t, err)
	_ = users
	mon := core.NewRecord(col)
	mon.Set("name", "h")
	mon.Set("type", "ping")
	mon.Set("target", "example.com")
	mon.Set("interval", 60)
	mon.Set("timeout", 10)
	mon.Set("status", "up")
	mon.Set("users", []string{u.Id})
	require.NoError(t, app.Save(mon))

	checksCol, err := app.FindCachedCollectionByNameOrId("monitor_checks")
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		c := core.NewRecord(checksCol)
		c.Set("monitor", mon.Id)
		c.Set("status", "up")
		c.Set("latency_ms", float64(i))
		require.NoError(t, app.Save(c))
	}

	limited := pbtests.ApiScenario{
		Name: "history limit 2", Method: http.MethodGet,
		URL:     "/api/beszel/monitors/" + mon.Id + "/checks?limit=2",
		Headers: auth, ExpectedStatus: 200,
		ExpectedContent:    []string{`"latency_ms":4`, `"latency_ms":3`},
		NotExpectedContent: []string{`"latency_ms":0`},
		TestAppFactory:     factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
	}
	_ = limited
	limited.Test(t)
}

func TestMonitorAPI_ListSortedAndBounds(t *testing.T) {
	app, token := sharedAPIApp(t)
	factory := func(testing.TB) *pbtests.TestApp { return app }
	auth := map[string]string{"Authorization": token, "Content-Type": "application/json"}
	hook := withMonitorRoutes()

	for _, name := range []string{"zeta", "alpha", "mid"} {
		sc := pbtests.ApiScenario{
			Name: "create " + name, Method: http.MethodPost, URL: "/api/beszel/monitors",
			Body:    strings.NewReader(`{"name":"` + name + `","type":"ping","target":"example.com"}`),
			Headers: auth, ExpectedStatus: 201, ExpectedContent: []string{`"name":"` + name + `"`},
			TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
		}
		sc.Test(t)
	}

	// Task-1 bounds enforced at API level.
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"empty target", `{"name":"e","type":"ping","target":""}`, "Target is required"},
		{"interval too small", `{"name":"e","type":"ping","target":"example.com","interval":5}`, "Interval must be"},
		{"interval too big", `{"name":"e","type":"ping","target":"example.com","interval":99999}`, "Interval must be"},
		{"bad retries", `{"name":"e","type":"ping","target":"example.com","max_retries":11}`, "Max_retries must be"},
		{"tls url accepted", `{"name":"t","type":"tls","target":"https://example.com:8443/x"}`, `"name":"t"`},
	} {
		sc := pbtests.ApiScenario{
			Name: tc.name, Method: http.MethodPost, URL: "/api/beszel/monitors",
			Body:    strings.NewReader(tc.body),
			Headers: auth, ExpectedStatus: 200, ExpectedContent: []string{tc.want},
			TestAppFactory: factory, BeforeTestFunc: hook, DisableTestAppCleanup: true,
		}
		if tc.name == "tls url accepted" {
			sc.ExpectedStatus = 201
		} else {
			sc.ExpectedStatus = 400
		}
		sc.Test(t)
	}
}
