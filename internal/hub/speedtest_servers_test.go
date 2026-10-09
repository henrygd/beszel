//go:build testing

package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSpeedtestServers(t *testing.T) {
	var requests atomic.Int32
	var lastSearch atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		lastSearch.Store(r.URL.Query().Get("search"))
		assert.Equal(t, "js", r.URL.Query().Get("engine"))
		if r.URL.Query().Get("search") == "broken" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Trimmed real API response; the second item has an invalid ID and is skipped.
		w.Write([]byte(`[
			{"url":"http://speedtest.ams.t-mobile.nl:8080/speedtest/upload.php","lat":"52.3667","lon":"4.9000","distance":40,"name":"Amsterdam","country":"Netherlands","cc":"NL","sponsor":"Odido","id":"52365","preferred":0},
			{"name":"Nowhere","country":"","sponsor":"Broken","id":"x"}
		]`))
	}))
	defer upstream.Close()
	previousURL := speedtestServersURL
	speedtestServersURL = upstream.URL
	t.Cleanup(func() { speedtestServersURL = previousURL })

	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)
	user, err := createTestUser(hub)
	require.NoError(t, err)

	handler := speedtestServersMux(t, hub)
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	get := func(search string) (int, string) {
		request := httptest.NewRequest(http.MethodGet, "/api/beszel/speedtest/servers?search="+search, nil)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Code, response.Body.String()
	}

	status, body := get("Amsterdam")
	require.Equal(t, http.StatusOK, status, body)
	var servers []speedtestServer
	require.NoError(t, json.Unmarshal([]byte(body), &servers))
	assert.Equal(t, []speedtestServer{{ID: 52365, Name: "Odido", Location: "Amsterdam"}}, servers)
	assert.Equal(t, "Amsterdam", lastSearch.Load())
	assert.Equal(t, int32(1), requests.Load())

	// Repeated searches are served from the cache, case-insensitively.
	status, _ = get("amsterdam")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, int32(1), requests.Load())

	// Upstream errors are reported and not cached.
	status, _ = get("broken")
	assert.Equal(t, http.StatusBadGateway, status)
	get("broken")
	assert.Equal(t, int32(3), requests.Load())
}

func TestGetSpeedtestServersRequiresAuth(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)
	response := httptest.NewRecorder()
	speedtestServersMux(t, hub).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/beszel/speedtest/servers", nil))
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

// speedtestServersMux builds a router with the hub's custom API routes.
func speedtestServersMux(t *testing.T, hub *Hub) http.Handler {
	t.Helper()
	router, err := apis.NewRouter(hub)
	require.NoError(t, err)
	require.NoError(t, hub.registerApiRoutes(&core.ServeEvent{App: hub, Router: router}))
	handler, err := router.BuildMux()
	require.NoError(t, err)
	return handler
}
