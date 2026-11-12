//go:build testing

package hub_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/require"
)

const requestLimitMigration = "1792483200_enable_request_limits.go"

var signInRule = core.RateLimitRule{Label: "*:auth", MaxRequests: 10, Duration: 60}

func TestPasswordSignInIsRateLimited(t *testing.T) {
	testHub, handler := firstUserTestMux(t)
	defer testHub.Cleanup()

	settings := testHub.Settings()
	require.True(t, settings.RateLimits.Enabled)
	require.Equal(t, []core.RateLimitRule{signInRule}, settings.RateLimits.Rules)

	user, err := beszelTests.CreateUser(testHub, "signin@example.com", "password123")
	require.NoError(t, err)
	user.SetVerified(true)
	require.NoError(t, testHub.Save(user))

	for range signInRule.MaxRequests {
		response := postPasswordSignIn(handler, "signin@example.com", "password123")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}

	limited := postPasswordSignIn(handler, "signin@example.com", "password123")
	require.Equal(t, http.StatusTooManyRequests, limited.Code, limited.Body.String())
}

func TestRequestLimitMigration(t *testing.T) {
	pbDefaults := []core.RateLimitRule{
		{Label: "*:auth", MaxRequests: 2, Duration: 3},
		{Label: "*:create", MaxRequests: 20, Duration: 5},
		{Label: "/api/batch", MaxRequests: 3, Duration: 1},
		{Label: "/api/", MaxRequests: 300, Duration: 10},
	}
	// If this fails after a PocketBase upgrade, the defaults changed and the
	// migration's frozen list no longer removes them on new installs.
	require.Equal(t, pbDefaults, core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()}).Settings().RateLimits.Rules)

	tests := []struct {
		name     string
		rules    []core.RateLimitRule
		expected []core.RateLimitRule
	}{
		{
			name:     "untouched defaults are replaced by sign-in rule",
			rules:    pbDefaults,
			expected: []core.RateLimitRule{signInRule},
		},
		{
			name:     "no rules adds sign-in rule",
			rules:    nil,
			expected: []core.RateLimitRule{signInRule},
		},
		{
			name: "customized rules are kept",
			rules: []core.RateLimitRule{
				{Label: "*:auth", MaxRequests: 7, Duration: 11},
				{Label: "*:create", MaxRequests: 17, Duration: 9},
				{Label: "/api/batch", Audience: "@guest", MaxRequests: 3, Duration: 1},
				{Label: "/api/", MaxRequests: 600, Duration: 10},
			},
			expected: []core.RateLimitRule{
				{Label: "*:auth", MaxRequests: 7, Duration: 11},
				{Label: "*:create", MaxRequests: 17, Duration: 9},
				{Label: "/api/batch", Audience: "@guest", MaxRequests: 3, Duration: 1},
				{Label: "/api/", MaxRequests: 600, Duration: 10},
			},
		},
		{
			name:  "custom rules without auth rule get sign-in rule prepended",
			rules: append([]core.RateLimitRule{{Label: "*:create", MaxRequests: 17, Duration: 9}}, pbDefaults[2:]...),
			expected: []core.RateLimitRule{
				signInRule,
				{Label: "*:create", MaxRequests: 17, Duration: 9},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testHub, err := beszelTests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			defer testHub.Cleanup()

			settings := testHub.Settings()
			settings.RateLimits.Enabled = false
			settings.RateLimits.Rules = tt.rules
			require.NoError(t, testHub.Save(settings))

			_, err = testHub.DB().Delete(core.DefaultMigrationsTable, dbx.HashExp{"file": requestLimitMigration}).Execute()
			require.NoError(t, err)
			require.NoError(t, testHub.RunAppMigrations())

			settings = testHub.Settings()
			require.True(t, settings.RateLimits.Enabled)
			require.Equal(t, tt.expected, settings.RateLimits.Rules)
		})
	}
}

func TestRequestLimitMigrationRunsOnce(t *testing.T) {
	testHub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer testHub.Cleanup()

	// Administrator changes after the migration must survive another startup.
	adminRules := []core.RateLimitRule{{Label: "*:auth", MaxRequests: 13, Duration: 19}}
	settings := testHub.Settings()
	settings.RateLimits.Enabled = false
	settings.RateLimits.Rules = adminRules
	require.NoError(t, testHub.Save(settings))

	require.NoError(t, testHub.ClearBootstrap())
	require.NoError(t, testHub.Bootstrap())
	require.NoError(t, testHub.RunAllMigrations())

	settings = testHub.Settings()
	require.False(t, settings.RateLimits.Enabled)
	require.Equal(t, adminRules, settings.RateLimits.Rules)
}

func postPasswordSignIn(handler http.Handler, identity string, password string) *httptest.ResponseRecorder {
	body := jsonReader(map[string]string{
		"identity": identity,
		"password": password,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/collections/users/auth-with-password", body)
	request.RemoteAddr = "203.0.113.8:4242"
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
