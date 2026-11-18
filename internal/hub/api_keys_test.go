//go:build testing

package hub_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/tools/security"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/require"
)

func apiKeysRequest(handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, jsonReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func createApiKeyForTest(t *testing.T, handler http.Handler, sessionToken string, duration int64) string {
	t.Helper()
	res := apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys", sessionToken, map[string]any{"duration": duration})
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var data struct {
		Token   string `json:"token"`
		Expires string `json:"expires"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &data))
	require.NotEmpty(t, data.Token)
	expires, err := time.Parse(time.RFC3339, data.Expires)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().Add(time.Duration(duration)*time.Second), expires, time.Minute)
	return data.Token
}

func TestApiKeys(t *testing.T) {
	hub, handler := firstUserTestMux(t)
	t.Cleanup(hub.Cleanup)

	user, err := beszelTests.CreateUserWithRole(hub, "user@example.com", "password123", "user")
	require.NoError(t, err)
	// the users authRule requires verified=true for auth-refresh
	user.SetVerified(true)
	require.NoError(t, hub.Save(user))
	readonly, err := beszelTests.CreateUserWithRole(hub, "readonly@example.com", "password123", "readonly")
	require.NoError(t, err)
	superuser, err := beszelTests.CreateSuperuser(hub, "superuser@example.com", "password123")
	require.NoError(t, err)

	sessionToken, err := user.NewAuthToken()
	require.NoError(t, err)

	t.Run("requires auth", func(t *testing.T) {
		res := apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys", "", map[string]any{"duration": 3600})
		require.Equal(t, http.StatusUnauthorized, res.Code)
		res = apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys/revoke", "", nil)
		require.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("rejects invalid durations", func(t *testing.T) {
		for _, duration := range []int64{0, -1, 60, 367 * 24 * 3600} {
			res := apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys", sessionToken, map[string]any{"duration": duration})
			require.Equal(t, http.StatusBadRequest, res.Code, "duration %d", duration)
		}
	})

	t.Run("no expiry", func(t *testing.T) {
		res := apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys", sessionToken, map[string]any{"noExpiry": true})
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var data struct {
			Token   string `json:"token"`
			Expires string `json:"expires"`
		}
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &data))
		require.Empty(t, data.Expires)

		claims, err := security.ParseUnverifiedJWT(data.Token)
		require.NoError(t, err)
		exp, err := claims.GetExpirationTime()
		require.NoError(t, err)
		require.True(t, exp.After(time.Now().Add(99*365*24*time.Hour)), "exp %v", exp)

		res = apiKeysRequest(handler, http.MethodGet, "/api/beszel/info", data.Token, nil)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	})

	t.Run("rejects superusers", func(t *testing.T) {
		superuserToken, err := superuser.NewAuthToken()
		require.NoError(t, err)
		res := apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys", superuserToken, map[string]any{"duration": 3600})
		require.Equal(t, http.StatusForbidden, res.Code)
	})

	t.Run("readonly users can create keys", func(t *testing.T) {
		readonlyToken, err := readonly.NewAuthToken()
		require.NoError(t, err)
		createApiKeyForTest(t, handler, readonlyToken, 3600)
	})

	t.Run("key authenticates as the user and can't be refreshed or mint keys", func(t *testing.T) {
		apiKey := createApiKeyForTest(t, handler, sessionToken, 30*24*3600)

		res := apiKeysRequest(handler, http.MethodGet, "/api/beszel/info", apiKey, nil)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())

		// auth-refresh returns the same static token instead of a refreshable one
		res = apiKeysRequest(handler, http.MethodPost, "/api/collections/users/auth-refresh", apiKey, nil)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var refreshed struct {
			Token string `json:"token"`
		}
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &refreshed))
		require.Equal(t, apiKey, refreshed.Token)

		res = apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys", apiKey, map[string]any{"duration": 3600})
		require.Equal(t, http.StatusForbidden, res.Code)
		res = apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys/revoke", apiKey, nil)
		require.Equal(t, http.StatusForbidden, res.Code)

		// impersonation tokens are static too
		impersonateToken, err := user.NewStaticAuthToken(time.Hour)
		require.NoError(t, err)
		res = apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys", impersonateToken, map[string]any{"duration": 3600})
		require.Equal(t, http.StatusForbidden, res.Code)
	})

	t.Run("revoke invalidates existing tokens and returns a working session", func(t *testing.T) {
		apiKey := createApiKeyForTest(t, handler, sessionToken, 3600)

		res := apiKeysRequest(handler, http.MethodPost, "/api/beszel/api-keys/revoke", sessionToken, nil)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var data struct {
			Token string `json:"token"`
		}
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &data))
		require.NotEmpty(t, data.Token)

		for _, oldToken := range []string{apiKey, sessionToken} {
			res = apiKeysRequest(handler, http.MethodGet, "/api/beszel/info", oldToken, nil)
			require.Equal(t, http.StatusUnauthorized, res.Code)
		}

		res = apiKeysRequest(handler, http.MethodGet, "/api/beszel/info", data.Token, nil)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		// the new session token can create keys again
		createApiKeyForTest(t, handler, data.Token, 3600)
	})
}
