//go:build testing
// +build testing

package heartbeat_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/henrygd/beszel/internal/hub/heartbeat"
	beszeltests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("returns nil when app is missing", func(t *testing.T) {
		hb := heartbeat.New(nil, envGetter(map[string]string{
			"HEARTBEAT_URL": "https://heartbeat.example.com/ping",
		}))
		assert.Nil(t, hb)
	})

	t.Run("returns nil when URL is missing", func(t *testing.T) {
		app := newTestHub(t)
		hb := heartbeat.New(app.App, func(string) (string, bool) {
			return "", false
		})
		assert.Nil(t, hb)
	})

	t.Run("parses and normalizes config values", func(t *testing.T) {
		app := newTestHub(t)
		env := map[string]string{
			"HEARTBEAT_URL":      "  https://heartbeat.example.com/ping  ",
			"HEARTBEAT_INTERVAL": "90",
			"HEARTBEAT_METHOD":   "head",
		}
		getEnv := func(key string) (string, bool) {
			v, ok := env[key]
			return v, ok
		}

		hb := heartbeat.New(app.App, getEnv)
		require.NotNil(t, hb)
		cfg := hb.GetConfig()
		assert.Equal(t, "https://heartbeat.example.com/ping", cfg.URL)
		assert.Equal(t, 90, cfg.Interval)
		assert.Equal(t, http.MethodHead, cfg.Method)
	})
}

func TestSendGETDoesNotRequireAppOrDB(t *testing.T) {
	app := newTestHub(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Beszel-Heartbeat", r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hb := heartbeat.New(app.App, envGetter(map[string]string{
		"HEARTBEAT_URL":    server.URL,
		"HEARTBEAT_METHOD": "GET",
	}))
	require.NotNil(t, hb)

	require.NoError(t, hb.Send())
}

func TestSendReturnsErrorOnHTTPFailureStatus(t *testing.T) {
	app := newTestHub(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	hb := heartbeat.New(app.App, envGetter(map[string]string{
		"HEARTBEAT_URL":    server.URL,
		"HEARTBEAT_METHOD": "GET",
	}))
	require.NotNil(t, hb)

	err := hb.Send()
	require.Error(t, err)
	assert.ErrorContains(t, err, "heartbeat endpoint returned status 500")
}

func TestSendPOSTBuildsExpectedStatuses(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T, app *beszeltests.TestHub, user *core.Record)
		expectStatus   string
		expectMsgPart  string
		expectDown     int
		expectAlerts   int
		expectTotal    int
		expectUp       int
		expectPaused   int
		expectPending  int
		expectDownSumm int
	}{
		{
			name: "error when at least one system is down",
			setup: func(t *testing.T, app *beszeltests.TestHub, user *core.Record) {
				downSystem := createTestSystem(t, app, user.Id, "db-1", "10.0.0.1", "down")
				_ = createTestSystem(t, app, user.Id, "web-1", "10.0.0.2", "up")
				createTriggeredAlert(t, app, user.Id, downSystem.Id, "CPU", 95)
			},
			expectStatus:   "error",
			expectMsgPart:  "1 system(s) down",
			expectDown:     1,
			expectAlerts:   1,
			expectTotal:    2,
			expectUp:       1,
			expectDownSumm: 1,
		},
		{
			name: "warn when only alerts are triggered",
			setup: func(t *testing.T, app *beszeltests.TestHub, user *core.Record) {
				system := createTestSystem(t, app, user.Id, "api-1", "10.1.0.1", "up")
				createTriggeredAlert(t, app, user.Id, system.Id, "CPU", 90)
			},
			expectStatus:   "warn",
			expectMsgPart:  "1 alert(s) triggered",
			expectDown:     0,
			expectAlerts:   1,
			expectTotal:    1,
			expectUp:       1,
			expectDownSumm: 0,
		},
		{
			name: "ok when no down systems and no alerts",
			setup: func(t *testing.T, app *beszeltests.TestHub, user *core.Record) {
				_ = createTestSystem(t, app, user.Id, "node-1", "10.2.0.1", "up")
				_ = createTestSystem(t, app, user.Id, "node-2", "10.2.0.2", "paused")
				_ = createTestSystem(t, app, user.Id, "node-3", "10.2.0.3", "pending")
			},
			expectStatus:   "ok",
			expectMsgPart:  "All systems operational",
			expectDown:     0,
			expectAlerts:   0,
			expectTotal:    3,
			expectUp:       1,
			expectPaused:   1,
			expectPending:  1,
			expectDownSumm: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestHub(t)
			user := createTestUser(t, app)
			tt.setup(t, app, user)

			type requestCapture struct {
				method      string
				userAgent   string
				contentType string
				payload     heartbeat.Payload
			}

			captured := make(chan requestCapture, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var payload heartbeat.Payload
				require.NoError(t, json.Unmarshal(body, &payload))
				captured <- requestCapture{
					method:      r.Method,
					userAgent:   r.Header.Get("User-Agent"),
					contentType: r.Header.Get("Content-Type"),
					payload:     payload,
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			hb := heartbeat.New(app.App, envGetter(map[string]string{
				"HEARTBEAT_URL":    server.URL,
				"HEARTBEAT_METHOD": "POST",
			}))
			require.NotNil(t, hb)
			require.NoError(t, hb.Send())

			req := <-captured
			assert.Equal(t, http.MethodPost, req.method)
			assert.Equal(t, "Beszel-Heartbeat", req.userAgent)
			assert.Equal(t, "application/json", req.contentType)

			assert.Equal(t, tt.expectStatus, req.payload.Status)
			assert.Contains(t, req.payload.Msg, tt.expectMsgPart)
			assert.Equal(t, tt.expectDown, len(req.payload.Down))
			assert.Equal(t, tt.expectAlerts, len(req.payload.Alerts))
			assert.Equal(t, tt.expectTotal, req.payload.Systems.Total)
			assert.Equal(t, tt.expectUp, req.payload.Systems.Up)
			assert.Equal(t, tt.expectDownSumm, req.payload.Systems.Down)
			assert.Equal(t, tt.expectPaused, req.payload.Systems.Paused)
			assert.Equal(t, tt.expectPending, req.payload.Systems.Pending)
		})
	}
}

func newTestHub(t *testing.T) *beszeltests.TestHub {
	t.Helper()
	app, err := beszeltests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

func createTestUser(t *testing.T, app *beszeltests.TestHub) *core.Record {
	t.Helper()
	user, err := beszeltests.CreateUser(app.App, "admin@example.com", "password123")
	require.NoError(t, err)
	return user
}

func createTestSystem(t *testing.T, app *beszeltests.TestHub, userID, name, host, status string) *core.Record {
	t.Helper()
	system, err := beszeltests.CreateRecord(app.App, "systems", map[string]any{
		"name":   name,
		"host":   host,
		"port":   "45876",
		"users":  []string{userID},
		"status": status,
	})
	require.NoError(t, err)
	return system
}

func createTriggeredAlert(t *testing.T, app *beszeltests.TestHub, userID, systemID, name string, threshold float64) *core.Record {
	t.Helper()
	alert, err := beszeltests.CreateRecord(app.App, "alerts", map[string]any{
		"name":      name,
		"system":    systemID,
		"user":      userID,
		"value":     threshold,
		"min":       0,
		"triggered": true,
	})
	require.NoError(t, err)
	return alert
}

func envGetter(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}
