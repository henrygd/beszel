//go:build testing

package hub_test

import (
	"github.com/pocketbase/dbx"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/hub"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

func TestPasswordSignInIsRateLimited(t *testing.T) {
	testHub, handler := firstUserTestMux(t)
	defer testHub.Cleanup()

	settings := testHub.Settings()
	require.True(t, settings.RateLimits.Enabled)
	rule, ok := settings.RateLimits.FindRateLimitRule([]string{"*:auth"})
	require.True(t, ok)
	require.LessOrEqual(t, rule.MaxRequests, 5)
	require.GreaterOrEqual(t, rule.Duration, int64(1))

	user, err := beszelTests.CreateUser(testHub, "signin@example.com", "password123")
	require.NoError(t, err)
	user.SetVerified(true)
	require.NoError(t, testHub.Save(user))

	for attempt := 1; attempt <= rule.MaxRequests; attempt++ {
		response := postPasswordSignIn(handler, "signin@example.com", "password123")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}

	limited := postPasswordSignIn(handler, "signin@example.com", "password123")
	require.Equal(t, http.StatusTooManyRequests, limited.Code, limited.Body.String())
	time.Sleep(time.Duration(rule.Duration)*time.Second + 100*time.Millisecond)
	recovered := postPasswordSignIn(handler, "signin@example.com", "password123")
	require.Equal(t, http.StatusOK, recovered.Code, recovered.Body.String())
}

func TestRequestLimitMigration(t *testing.T) {
	for _, custom := range []bool{false, true} {
		name := "missing sign-in rule"
		if custom {
			name = "custom sign-in rule"
		}
		t.Run(name, func(t *testing.T) {
			testHub, err := beszelTests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			defer testHub.Cleanup()
			settings := testHub.Settings()
			settings.RateLimits.Enabled = false
			settings.RateLimits.Rules = []core.RateLimitRule{{Label: "*:create", MaxRequests: 17, Duration: 9}}
			if custom {
				settings.RateLimits.Rules = append(settings.RateLimits.Rules, core.RateLimitRule{Label: "*:auth", MaxRequests: 7, Duration: 11})
			}
			require.NoError(t, testHub.Save(settings))
			_, err = testHub.DB().Delete(core.DefaultMigrationsTable, dbx.HashExp{"file": "1792483200_enable_request_limits.go"}).Execute()
			require.NoError(t, err)
			require.NoError(t, testHub.RunAppMigrations())
			settings = testHub.Settings()
			require.True(t, settings.RateLimits.Enabled)
			rule, ok := settings.RateLimits.FindRateLimitRule([]string{"*:auth"})
			require.True(t, ok)
			if custom {
				require.Equal(t, 7, rule.MaxRequests)
				require.EqualValues(t, 11, rule.Duration)
			} else {
				require.Equal(t, 2, rule.MaxRequests)
				require.EqualValues(t, 3, rule.Duration)
			}
			other, ok := settings.RateLimits.FindRateLimitRule([]string{"*:create"})
			require.True(t, ok)
			require.Equal(t, 17, other.MaxRequests)

			// Administrator changes after migration must survive another startup.
			settings.RateLimits.Enabled = false
			settings.RateLimits.Rules = []core.RateLimitRule{{Label: "*:auth", MaxRequests: 13, Duration: 19}}
			require.NoError(t, testHub.Save(settings))
			require.NoError(t, testHub.ClearBootstrap())
			require.NoError(t, testHub.Bootstrap())
			require.NoError(t, testHub.RunAllMigrations())
			hub.NewHub(testHub.TestApp)
			settings = testHub.Settings()
			require.False(t, settings.RateLimits.Enabled)
			require.Equal(t, []core.RateLimitRule{{Label: "*:auth", MaxRequests: 13, Duration: 19}}, settings.RateLimits.Rules)
		})
	}
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
