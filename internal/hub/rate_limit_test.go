//go:build testing

package hub_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

	_, err := beszelTests.CreateUser(testHub, "signin@example.com", "password123")
	require.NoError(t, err)

	for attempt := 1; attempt <= rule.MaxRequests; attempt++ {
		response := postPasswordSignIn(handler, "signin@example.com", "password123")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}

	limited := postPasswordSignIn(handler, "signin@example.com", "password123")
	require.Equal(t, http.StatusTooManyRequests, limited.Code, limited.Body.String())
}

func TestMissingSignInRuleIsRestored(t *testing.T) {
	testHub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer testHub.Cleanup()

	settings := testHub.Settings()
	kept := make([]core.RateLimitRule, 0, len(settings.RateLimits.Rules))
	for _, rule := range settings.RateLimits.Rules {
		if rule.Label != "*:auth" {
			kept = append(kept, rule)
		}
	}
	settings.RateLimits.Rules = kept
	settings.RateLimits.Enabled = false
	require.NoError(t, testHub.Save(settings))

	hub.NewHub(testHub.TestApp)

	restored := testHub.Settings()
	require.True(t, restored.RateLimits.Enabled)
	_, ok := restored.RateLimits.FindRateLimitRule([]string{"*:auth"})
	require.True(t, ok)
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
