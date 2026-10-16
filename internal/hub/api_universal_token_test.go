//go:build testing

package hub_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestUniversalTokenJSONRequests(t *testing.T) {
	hub, handler := firstUserTestMux(t)
	defer hub.Cleanup()
	user, err := beszelTests.CreateUser(hub, "token-user@example.com", "password123")
	require.NoError(t, err)
	userToken, err := user.NewAuthToken()
	require.NoError(t, err)

	request := func(t *testing.T, method, path, body, auth string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	decode := func(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		var data map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &data))
		return data
	}
	const path = "/api/beszel/universal-token"
	const token = "dummy-universal-token-json-request"

	t.Run("enable ephemeral token through the request body", func(t *testing.T) {
		data := decode(t, request(t, http.MethodPost, path, `{"token":"`+token+`","enable":1}`, userToken))
		require.Equal(t, token, data["token"])
		require.Equal(t, true, data["active"])
		require.Equal(t, false, data["permanent"])
	})
	t.Run("read current state without a token in the URL", func(t *testing.T) {
		data := decode(t, request(t, http.MethodPost, path, `{}`, userToken))
		require.Equal(t, token, data["token"])
		require.Equal(t, true, data["active"])
	})
	t.Run("make the token permanent through the request body", func(t *testing.T) {
		data := decode(t, request(t, http.MethodPost, path, `{"token":"`+token+`","enable":1,"permanent":1}`, userToken))
		require.Equal(t, token, data["token"])
		require.Equal(t, true, data["permanent"])
	})
	t.Run("preserve token ownership across users", func(t *testing.T) {
		other, err := beszelTests.CreateUser(hub, "other-token-user@example.com", "password123")
		require.NoError(t, err)
		otherToken, err := other.NewAuthToken()
		require.NoError(t, err)
		data := decode(t, request(t, http.MethodPost, path, `{}`, otherToken))
		require.NotEqual(t, token, data["token"])
		data = decode(t, request(t, http.MethodPost, path, `{"token":"`+token+`"}`, otherToken))
		require.Equal(t, false, data["active"])
		decode(t, request(t, http.MethodPost, path, `{"enable":0}`, otherToken))
		data = decode(t, request(t, http.MethodPost, path, `{}`, userToken))
		require.Equal(t, token, data["token"])
		require.Equal(t, true, data["active"])
	})
	t.Run("disable the permanent token through the request body", func(t *testing.T) {
		data := decode(t, request(t, http.MethodPost, path, `{"enable":0}`, userToken))
		require.Equal(t, false, data["active"])
		data = decode(t, request(t, http.MethodPost, path, `{}`, userToken))
		require.Equal(t, false, data["active"])
		require.Equal(t, false, data["permanent"])
	})
	t.Run("keep legacy GET requests compatible and uncached", func(t *testing.T) {
		data := decode(t, request(t, http.MethodGet, path+"?enable=1&permanent=1&token=legacy-dummy-token", "", userToken))
		require.Equal(t, "legacy-dummy-token", data["token"])
		require.Equal(t, true, data["active"])
		data = decode(t, request(t, http.MethodGet, path, "", userToken))
		require.Equal(t, "legacy-dummy-token", data["token"])
	})

	for _, body := range []string{`{`, `{"enable":"yes"}`, `{"enable":2}`, `{"permanent":2}`, `{"enable":-2}`} {
		t.Run("reject invalid request "+body, func(t *testing.T) {
			response := request(t, http.MethodPost, path, body, userToken)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		})
	}
	t.Run("require authentication", func(t *testing.T) {
		response := request(t, http.MethodPost, path, `{"enable":1}`, "")
		require.Equal(t, http.StatusUnauthorized, response.Code)
	})
	t.Run("reject readonly users", func(t *testing.T) {
		user, err := beszelTests.CreateUserWithRole(hub, "readonly-token@example.com", "password123", "readonly")
		require.NoError(t, err)
		auth, err := user.NewAuthToken()
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, request(t, http.MethodPost, path, `{"enable":1}`, auth).Code)
	})
	t.Run("reject superusers", func(t *testing.T) {
		user, err := beszelTests.CreateSuperuser(hub, "superuser-token@example.com", "password123")
		require.NoError(t, err)
		auth, err := user.NewAuthToken()
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, request(t, http.MethodPost, path, `{"enable":1}`, auth).Code)
	})
}
