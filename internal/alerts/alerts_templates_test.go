//go:build testing

package alerts_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTemplate(t *testing.T) {
	vars := map[string]string{"system.name": "web-1", "state": "down", "empty": ""}
	assert.Equal(t, "web-1 is down", alerts.RenderTemplate("{system.name} is {state}", vars))
	assert.Equal(t, "downdown", alerts.RenderTemplate("{state}{state}", vars))
	assert.Equal(t, "[]", alerts.RenderTemplate("[{empty}]", vars), "known but empty values render empty")
	assert.Equal(t, "keep {unknown} {Bad.Case} { state }", alerts.RenderTemplate("keep {unknown} {Bad.Case} { state }", vars),
		"unknown or malformed placeholders are left untouched")
	// inline wording map
	assert.Equal(t, "KOPTU", alerts.RenderTemplate("{state:down=KOPTU,up=geri geldi}", vars))
	assert.Equal(t, "geri geldi", alerts.RenderTemplate("{state: down = KOPTU , up = geri geldi }", map[string]string{"state": "up"}), "keys and texts are trimmed")
	assert.Equal(t, "web-1", alerts.RenderTemplate("{system.name:down=x}", vars), "values without a matching key are unchanged")
	assert.Equal(t, "{nope:down=x}", alerts.RenderTemplate("{nope:down=x}", vars), "unknown placeholders keep their map")
	assert.Equal(t, "down", alerts.RenderTemplate("{state:}", vars), "an empty map is ignored")
}

func TestFormatUptime(t *testing.T) {
	assert.Equal(t, "5m", alerts.FormatUptime(5*60))
	assert.Equal(t, "2h 5m", alerts.FormatUptime(2*3600+5*60))
	assert.Equal(t, "3d 4h 12m", alerts.FormatUptime(3*86400+4*3600+12*60))
}

func TestValidateNotificationSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings alerts.UserNotificationSettings
		wantErr  string
	}{
		{"empty", alerts.UserNotificationSettings{}, ""},
		{"valid", alerts.UserNotificationSettings{
			Templates: map[string]alerts.NotificationTemplate{"status": {Title: "{system.name}"}},
			Timezone:  "Europe/Istanbul",
		}, ""},
		{"unknown kind", alerts.UserNotificationSettings{
			Templates: map[string]alerts.NotificationTemplate{"bogus": {Title: "x"}},
		}, "unknown notification template kind"},
		{"too long", alerts.UserNotificationSettings{
			Templates: map[string]alerts.NotificationTemplate{"status": {Body: strings.Repeat("x", 2001)}},
		}, "exceeds 2000"},
		{"exactly at the limit (multibyte)", alerts.UserNotificationSettings{
			Templates: map[string]alerts.NotificationTemplate{"status": {Body: strings.Repeat("ş", 2000)}},
		}, ""},
		{"one over the limit (multibyte)", alerts.UserNotificationSettings{
			Templates: map[string]alerts.NotificationTemplate{"status": {Body: strings.Repeat("ş", 2001)}},
		}, "exceeds 2000"},
		{"bad timezone", alerts.UserNotificationSettings{Timezone: "Mars/Olympus"}, "invalid timezone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := alerts.ValidateNotificationSettings(tc.settings)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

// userSettingsRecord returns the user's user_settings record, creating it if needed.
func userSettingsRecord(t *testing.T, app core.App, userID string) *core.Record {
	t.Helper()
	record, err := app.FindFirstRecordByFilter("user_settings", "user={:user}", dbx.Params{"user": userID})
	if err == nil {
		return record
	}
	record, err = beszelTests.CreateRecord(app, "user_settings", map[string]any{"user": userID})
	require.NoError(t, err)
	return record
}

func saveUserSettings(t *testing.T, app core.App, userID string, settings alerts.UserNotificationSettings) {
	t.Helper()
	record := userSettingsRecord(t, app, userID)
	record.Set("settings", settings)
	require.NoError(t, app.Save(record))
}

func TestSendAlertUsesUserTemplates(t *testing.T) {
	hub, user := beszelTests.GetHubWithUser(t)
	defer hub.Cleanup()
	am := alerts.NewTestAlertManagerWithoutWorker(hub)

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	system := systems[0]
	name := system.GetString("name")
	emails := []string{"alerts@example.com"}

	base := alerts.AlertMessageData{
		UserID:   user.Id,
		SystemID: system.Id,
		Kind:     alerts.NotificationKindStatus,
		State:    "down",
		Title:    "Connection to " + name + " is down \U0001F534",
		Message:  "Connection to " + name + " is down ",
		Link:     "https://hub.example.com/system/" + system.Id,
		LinkText: "View " + name,
	}

	t.Run("built-in text when no template is set", func(t *testing.T) {
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{Emails: emails})
		require.NoError(t, am.SendAlert(base))
		msg := hub.TestMailer.LastMessage()
		assert.Equal(t, base.Title, msg.Subject)
		assert.Equal(t, base.Message+"\n\n"+base.Link, msg.Text)
	})

	t.Run("kind template with inline map, timezone, system vars and link", func(t *testing.T) {
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Emails:   emails,
			Timezone: "Europe/Istanbul",
			Templates: map[string]alerts.NotificationTemplate{
				"status": {
					Title: "  [{system.name}]   bağlantı\n{state:down=KOPTU,up=geri geldi} ",
					Body:  "Cihaz: {system.name} ({system.host})\nZaman: {time_iso} {timezone}\n{link}\n{unknown}",
				},
			},
		})
		require.NoError(t, am.SendAlert(base))
		msg := hub.TestMailer.LastMessage()
		assert.Equal(t, "["+name+"] bağlantı KOPTU", msg.Subject, "title is rendered and collapsed to one line")
		assert.Contains(t, msg.Text, "Cihaz: "+name+" ("+system.GetString("host")+")")
		loc, _ := time.LoadLocation("Europe/Istanbul")
		assert.Contains(t, msg.Text, time.Now().In(loc).Format("-07:00")+" Europe/Istanbul", "timestamp uses the user's timezone")
		assert.Equal(t, 1, strings.Count(msg.Text, base.Link), "link is not appended a second time")
		assert.True(t, strings.HasSuffix(msg.Text, "{unknown}"), "unknown placeholders stay visible")
	})

	t.Run("kind variables and inline metric map", func(t *testing.T) {
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Emails:    emails,
			Templates: map[string]alerts.NotificationTemplate{"system": {Body: "{message}\n{metric:CPU=İşlemci} {value}{unit} > {threshold} ({state})"}},
		})
		data := base
		data.Kind = alerts.NotificationKindSystem
		data.State = "above"
		data.Title = name + " CPU above threshold"
		data.Message = "CPU averaged 92.50% for the previous 10 minutes."
		data.Vars = map[string]string{"metric": "CPU", "value": "92.50", "unit": "%", "threshold": "80", "minutes": "10"}
		require.NoError(t, am.SendAlert(data))
		msg := hub.TestMailer.LastMessage()
		assert.Equal(t, data.Title, msg.Subject, "title keeps the built-in text")
		assert.Equal(t, data.Message+"\nİşlemci 92.50% > 80 (above)", msg.Text)
		assert.NotContains(t, msg.Text, base.Link, "custom body owns link placement")
	})

	t.Run("templates of other kinds do not apply", func(t *testing.T) {
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Emails:    emails,
			Templates: map[string]alerts.NotificationTemplate{"system": {Title: "SYS", Body: "SYS BODY"}},
		})
		require.NoError(t, am.SendAlert(base))
		msg := hub.TestMailer.LastMessage()
		assert.Equal(t, base.Title, msg.Subject)
		assert.Equal(t, base.Message+"\n\n"+base.Link, msg.Text)
	})

	t.Run("title-only template keeps built-in body and appended link", func(t *testing.T) {
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Emails:    emails,
			Templates: map[string]alerts.NotificationTemplate{"status": {Title: "T {state} {system.name}"}},
		})
		require.NoError(t, am.SendAlert(base))
		msg := hub.TestMailer.LastMessage()
		assert.Equal(t, "T down "+name, msg.Subject)
		assert.Equal(t, base.Message+"\n\n"+base.Link, msg.Text)
	})

	t.Run("invalid timezone falls back to hub time", func(t *testing.T) {
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Emails:    emails,
			Timezone:  "Nope/Zone",
			Templates: map[string]alerts.NotificationTemplate{"status": {Body: "tz={timezone}"}},
		})
		require.NoError(t, am.SendAlert(base))
		zone, _ := time.Now().In(time.Local).Zone()
		assert.Equal(t, "tz="+zone, hub.TestMailer.LastMessage().Text, "falls back to the hub zone abbreviation, never the literal Local")
	})

	t.Run("system_details values and inline maps for state variables", func(t *testing.T) {
		col, err := hub.FindCollectionByNameOrId("system_details")
		require.NoError(t, err)
		details := core.NewRecord(col)
		details.Set("id", system.Id)
		details.Set("system", system.Id)
		details.Set("hostname", "gebze-ds05")
		details.Set("kernel", "6.8.0-45-generic")
		details.Set("cores", 8)
		details.Set("cpu", "Intel Core i7-1165G7")
		details.Set("os_name", "Ubuntu 24.04")
		details.Set("arch", "amd64")
		require.NoError(t, hub.SaveNoValidate(details))
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Emails: emails,
			Templates: map[string]alerts.NotificationTemplate{
				"smart": {Body: "{system.hostname}|{system.os}|{system.kernel}|{system.cpu}|{system.cores}|{system.arch}|{old_state:PASSED=SAĞLAM}>{new_state:FAILED=ARIZALI} ({state:FAILED=ARIZALI}) {metric:Memory=Bellek}"},
			},
		})
		data := base
		data.Kind = alerts.NotificationKindSmart
		data.State = "FAILED"
		data.Vars = map[string]string{"old_state": "PASSED", "new_state": "FAILED", "metric": "Memory"}
		require.NoError(t, am.SendAlert(data))
		assert.Equal(t, "gebze-ds05|Ubuntu 24.04|6.8.0-45-generic|Intel Core i7-1165G7|8|amd64|SAĞLAM>ARIZALI (ARIZALI) Bellek", hub.TestMailer.LastMessage().Text)
	})

	t.Run("missing system renders known placeholders empty", func(t *testing.T) {
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Emails:    emails,
			Templates: map[string]alerts.NotificationTemplate{"status": {Body: "[{system.name}|{system.os}|{nope}]"}},
		})
		data := base
		data.SystemID = "doesnotexist000"
		require.NoError(t, am.SendAlert(data))
		assert.Equal(t, "[||{nope}]", hub.TestMailer.LastMessage().Text)
	})

	t.Run("rendered output is capped", func(t *testing.T) {
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Emails:    emails,
			Templates: map[string]alerts.NotificationTemplate{"status": {Title: strings.Repeat("{message}", 60), Body: strings.Repeat("{message} ", 200)}},
		})
		data := base
		data.Message = strings.Repeat("x", 1500)
		require.NoError(t, am.SendAlert(data))
		msg := hub.TestMailer.LastMessage()
		assert.LessOrEqual(t, len([]rune(msg.Subject)), 500+len([]rune("\n…(truncated)")))
		assert.LessOrEqual(t, len([]rune(msg.Text)), 8000+len([]rune("\n…(truncated)")))
		assert.True(t, strings.HasSuffix(msg.Text, "…(truncated)"))
	})

	t.Run("webhook body respects custom link placement", func(t *testing.T) {
		var mu sync.Mutex
		var received string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			received = string(body)
			mu.Unlock()
		}))
		defer server.Close()
		// internal destinations require an admin owner
		user.Set("role", "admin")
		require.NoError(t, hub.Save(user))
		saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
			Webhooks:  []string{"generic+" + server.URL},
			Templates: map[string]alerts.NotificationTemplate{"status": {Body: "{system.name} {state} {link}"}},
		})
		require.NoError(t, am.SendAlert(base))
		mu.Lock()
		defer mu.Unlock()
		assert.Contains(t, received, name+" down "+base.Link)
		assert.Equal(t, 1, strings.Count(received, base.Link), "link is not appended again by the generic service fallback")
	})
}

func TestNotificationTemplatePreviewApi(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()
	hub.StartHub()

	user, err := beszelTests.CreateUser(hub, "tpl@example.com", "password")
	require.NoError(t, err)
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	_, err = beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "tpl-system",
		"users": []string{user.Id},
		"host":  "10.0.0.5",
	})
	require.NoError(t, err)
	saveUserSettings(t, hub, user.Id, alerts.UserNotificationSettings{
		Emails:    []string{"team@example.com"},
		Templates: map[string]alerts.NotificationTemplate{"status": {Body: "SAVED BODY {state}"}},
	})
	admin, err := beszelTests.CreateUserWithRole(hub, "tpladmin@example.com", "password", "admin")
	require.NoError(t, err)
	adminToken, err := admin.NewAuthToken()
	require.NoError(t, err)
	saveUserSettings(t, hub, admin.Id, alerts.UserNotificationSettings{Emails: []string{"ops@example.com", "noc@example.com"}})
	readonly, err := beszelTests.CreateUserWithRole(hub, "tplro@example.com", "password", "readonly")
	require.NoError(t, err)
	readonlyToken, err := readonly.NewAuthToken()
	require.NoError(t, err)

	const url = "/api/beszel/notification-templates/preview"
	testAppFactory := func(t testing.TB) *pbTests.TestApp { return hub.TestApp }
	auth := map[string]string{"Authorization": token}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "no auth",
			Method:          http.MethodPost,
			URL:             url,
			Body:            jsonReader(map[string]any{"kind": "status", "title": "x"}),
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "unknown kind",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         auth,
			Body:            jsonReader(map[string]any{"kind": "bogus", "title": "x"}),
			ExpectedStatus:  400,
			ExpectedContent: []string{"Unknown notification kind"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "invalid timezone",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         auth,
			Body:            jsonReader(map[string]any{"kind": "status", "title": "x", "timezone": "Mars/Olympus"}),
			ExpectedStatus:  400,
			ExpectedContent: []string{"timezone: Mars/Olympus"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:    "renders system placeholders and the inline map",
			Method:  http.MethodPost,
			URL:     url,
			Headers: auth,
			Body: jsonReader(map[string]any{
				"kind":  "status",
				"title": "{system.name} {state:down=KOPTU,up=geri geldi}",
				"body":  "{system.host} {link} {timezone}",
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"title":"tpl-system KOPTU"`, "10.0.0.5", "/system/"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "system kind sample variables",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         auth,
			Body:            jsonReader(map[string]any{"kind": "system", "body": "{metric} {value}{unit} > {threshold}% for {minutes}m ({state})"}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"CPU 92.50% > 80% for 10m (above)"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "default is no longer a template kind",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         auth,
			Body:            jsonReader(map[string]any{"kind": "default", "title": "D"}),
			ExpectedStatus:  400,
			ExpectedContent: []string{"Unknown notification kind"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "empty template previews built-in text",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         auth,
			Body:            jsonReader(map[string]any{"kind": "container"}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"2 unhealthy containers on tpl-system"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "preview uses the submitted text, not the saved template",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         auth,
			Body:            jsonReader(map[string]any{"kind": "status", "title": "T {state}"}),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"body":"Connection to tpl-system is down "`, `"title":"T down"`},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "sendEmail from a regular user goes to the account address only",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         auth,
			Body:            jsonReader(map[string]any{"kind": "status", "title": "MAIL {system.name}", "body": "B {state}", "sendEmail": true}),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"title":"MAIL tpl-system"`},
			TestAppFactory:  testAppFactory,
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				msg := app.TestMailer.LastMessage()
				require.NotNil(t, msg)
				assert.Equal(t, "MAIL tpl-system", msg.Subject)
				assert.Equal(t, "B down", msg.Text)
				require.Len(t, msg.To, 1)
				assert.Equal(t, "tpl@example.com", msg.To[0].Address, "saved addresses must not be used by non-admins")
			},
		},
		{
			Name:            "admin sendEmail goes to the configured addresses",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         map[string]string{"Authorization": adminToken},
			Body:            jsonReader(map[string]any{"kind": "status", "title": "ADMIN {state}", "sendEmail": true}),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"title":"ADMIN down"`},
			TestAppFactory:  testAppFactory,
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				msg := app.TestMailer.LastMessage()
				require.NotNil(t, msg)
				assert.Equal(t, "ADMIN down", msg.Subject)
				require.Len(t, msg.To, 2)
				assert.Equal(t, "ops@example.com", msg.To[0].Address)
			},
		},
		{
			Name:            "readonly user cannot send test emails",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         map[string]string{"Authorization": readonlyToken},
			Body:            jsonReader(map[string]any{"kind": "status", "title": "RO", "sendEmail": true}),
			ExpectedStatus:  403,
			ExpectedContent: []string{"Read-only"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "readonly user can still preview",
			Method:          http.MethodPost,
			URL:             url,
			Headers:         map[string]string{"Authorization": readonlyToken},
			Body:            jsonReader(map[string]any{"kind": "status", "title": "RO {state}"}),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"title":"RO down"`},
			TestAppFactory:  testAppFactory,
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestUserSettingsTemplateValidationApi(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()
	hub.StartHub()

	user, err := beszelTests.CreateUser(hub, "tplsettings@example.com", "password")
	require.NoError(t, err)
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	record := userSettingsRecord(t, hub, user.Id)

	url := "/api/collections/user_settings/records/" + record.Id
	testAppFactory := func(t testing.TB) *pbTests.TestApp { return hub.TestApp }
	auth := map[string]string{"Authorization": token}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:    "create request is validated too",
			Method:  http.MethodPost,
			URL:     "/api/collections/user_settings/records",
			Headers: auth,
			Body: jsonReader(map[string]any{"user": user.Id, "settings": map[string]any{
				"notificationTemplates": map[string]any{"bogus": map[string]any{"title": "x"}},
			}}),
			ExpectedStatus:  400,
			ExpectedContent: []string{"notification template kind: bogus"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:    "unknown template kind is rejected",
			Method:  http.MethodPatch,
			URL:     url,
			Headers: auth,
			Body: jsonReader(map[string]any{"settings": map[string]any{
				"emails": []string{}, "notificationTemplates": map[string]any{"bogus": map[string]any{"title": "x"}},
			}}),
			ExpectedStatus:  400,
			ExpectedContent: []string{"notification template kind: bogus"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:    "oversized template is rejected",
			Method:  http.MethodPatch,
			URL:     url,
			Headers: auth,
			Body: jsonReader(map[string]any{"settings": map[string]any{
				"notificationTemplates": map[string]any{"status": map[string]any{"body": strings.Repeat("x", 2001)}},
			}}),
			ExpectedStatus:  400,
			ExpectedContent: []string{"exceeds 2000"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "invalid timezone is rejected",
			Method:          http.MethodPatch,
			URL:             url,
			Headers:         auth,
			Body:            jsonReader(map[string]any{"settings": map[string]any{"notificationTimezone": "Mars/Olympus"}}),
			ExpectedStatus:  400,
			ExpectedContent: []string{"timezone: Mars/Olympus"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:    "wrong template shape is rejected",
			Method:  http.MethodPatch,
			URL:     url,
			Headers: auth,
			Body: jsonReader(map[string]any{"settings": map[string]any{
				"notificationTemplates": []string{"not", "an", "object"},
			}}),
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid notification template settings"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:    "valid templates are saved",
			Method:  http.MethodPatch,
			URL:     url,
			Headers: auth,
			Body: jsonReader(map[string]any{"settings": map[string]any{
				"emails":                []string{"a@example.com"},
				"notificationTemplates": map[string]any{"status": map[string]any{"title": "[{system.name}] {state}"}},
				"notificationTimezone":  "Europe/Istanbul",
			}}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"Europe/Istanbul", "[{system.name}] {state}"},
			TestAppFactory:  testAppFactory,
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
