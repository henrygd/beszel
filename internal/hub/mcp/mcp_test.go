//go:build testing

package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// mcpServer builds the hub router mux (including the /mcp route) and returns
// a real HTTP server for it.
func mcpServer(t *testing.T, hub *beszelTests.TestHub) *httptest.Server {
	t.Helper()
	baseRouter, err := apis.NewRouter(hub.TestApp)
	require.NoError(t, err)
	serveEvent := new(core.ServeEvent)
	serveEvent.App = hub.TestApp
	serveEvent.Router = baseRouter
	require.NoError(t, hub.TestApp.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		return e.Next()
	}))
	mux, err := serveEvent.Router.BuildMux()
	require.NoError(t, err)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// mcpClient connects an MCP client to the hub's /mcp endpoint with the
// authorization token set on every request, like real MCP clients do.
func mcpClient(t *testing.T, serverURL string, token string) *mcpsdk.ClientSession {
	t.Helper()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "beszel-mcp-test", Version: "1.0.0"}, nil)
	transport := &mcpsdk.StreamableClientTransport{
		Endpoint: serverURL + "/mcp",
		HTTPClient: &http.Client{Transport: authRoundTripper{
			token: token,
			base:  http.DefaultTransport,
		}},
		DisableStandaloneSSE: true,
	}
	session, err := client.Connect(context.Background(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (a authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if a.token != "" {
		req.Header.Set("Authorization", a.token)
	}
	return a.base.RoundTrip(req)
}

// createSystem adds a system owned by userId with the given name.
// Values are applied after creation because the hub's create hook resets
// info and status for new system records.
func createSystem(t *testing.T, hub *beszelTests.TestHub, name string, userId string) *core.Record {
	t.Helper()
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  name,
		"host":  "127.0.0.1",
		"port":  "33914",
		"users": []string{userId},
	})
	require.NoError(t, err)
	// hardware details reported by the agent (id matches the system id)
	_, err = beszelTests.CreateRecord(hub, "system_details", map[string]any{
		"id":       system.Id,
		"system":   system.Id,
		"hostname": name,
		"cpu":      "Test CPU 9000",
		"cores":    8,
		"threads":  16,
		"memory":   8589934592,
		"arch":     "x86_64",
		"os_name":  "Debian GNU/Linux 12",
	})
	require.NoError(t, err)
	system.Set("status", "up")
	// "b" is the deprecated Info.Bandwidth (MB/s) field, exercising the legacy
	// fallback path in newSystemSummary (BandwidthBytes unset)
	system.Set("info", map[string]any{"cpu": 42, "mp": 51, "dp": 33, "b": 1.5})
	require.NoError(t, hub.Save(system))
	reloaded, err := hub.FindRecordById("systems", system.Id)
	require.NoError(t, err)
	return reloaded
}

// createStatsRecord adds a system_stats record for the system, with created
// set to the given number of minutes ago for deterministic ordering.
func createStatsRecord(t *testing.T, hub *beszelTests.TestHub, systemId string, stats map[string]any, minutesAgo int) {
	t.Helper()
	raw, err := json.Marshal(stats)
	require.NoError(t, err)
	created := time.Now().UTC().Add(-time.Duration(minutesAgo) * time.Minute).Format(types.DefaultDateLayout)
	_, err = beszelTests.CreateRecord(hub, "system_stats", map[string]any{
		"system":  systemId,
		"type":    "1m",
		"stats":   string(raw),
		"created": created,
	})
	require.NoError(t, err)
}

type testUser struct {
	Record *core.Record
	Token  string
}

// newTestUser creates a user and their auth token.
func newTestUser(t *testing.T, hub *beszelTests.TestHub, email string) testUser {
	t.Helper()
	user, err := beszelTests.CreateUser(hub, email, "password")
	require.NoError(t, err)
	token, err := user.NewAuthToken()
	require.NoError(t, err)
	return testUser{Record: user, Token: token}
}

// setupHub creates a test hub with the MCP route registered, two users and
// three systems (two for the first user), returning an httptest server for it.
func setupHub(t *testing.T) (*beszelTests.TestHub, testUser, testUser, *httptest.Server, *core.Record) {
	return setupHubWithShareAll(t, "false")
}

// setupHubWithShareAll is setupHub with an explicit SHARE_ALL_SYSTEMS value.
func setupHubWithShareAll(t *testing.T, shareAllSystems string) (*beszelTests.TestHub, testUser, testUser, *httptest.Server, *core.Record) {
	t.Helper()
	t.Setenv("BESZEL_HUB_MCP", "true")
	t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", shareAllSystems)
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(hub.Cleanup)
	hub.StartHub()
	server := mcpServer(t, hub)

	user := newTestUser(t, hub, "mcp-user@example.com")
	other := newTestUser(t, hub, "mcp-other@example.com")
	userSystem := createSystem(t, hub, "user-system-1", user.Record.Id)
	createSystem(t, hub, "user-system-2", user.Record.Id)
	createSystem(t, hub, "other-system", other.Record.Id)
	return hub, user, other, server, userSystem
}

// callTool invokes a tool and returns its structured content, asserting the
// call did not end in a tool error.
func callTool(t *testing.T, session *mcpsdk.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.NotNil(t, result.StructuredContent)
	return result.StructuredContent.(map[string]any)
}

// callToolError invokes a tool expecting a tool error result, returned as text.
func callToolError(t *testing.T, session *mcpsdk.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)
	text := ""
	for _, content := range result.Content {
		if textContent, ok := content.(*mcpsdk.TextContent); ok {
			text += textContent.Text
		}
	}
	return text
}

func TestMcpNotRegistered(t *testing.T) {
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(hub.Cleanup)
	hub.StartHub()
	server := mcpServer(t, hub)
	res, err := http.Post(server.URL+"/mcp", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestMcpUnauthorized(t *testing.T) {
	hub, user, _, server, _ := setupHub(t)
	hub.Settings().Meta.AppURL = "https://beszel.example.com/"
	expiredToken, err := user.Record.NewStaticAuthToken(time.Millisecond)
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond) // token exp has second precision

	const settingsPage = "Settings > API Keys (https://beszel.example.com/settings/api-keys)"
	tests := []struct {
		name          string
		authorization string
		message       string
		challenge     string
	}{
		{
			name:      "missing",
			message:   `Missing API key. Create one in ` + settingsPage + ` and send it in the Authorization header as "Bearer <key>".`,
			challenge: `Bearer realm="beszel"`,
		},
		{
			name:          "invalid",
			authorization: "Bearer not-a-token",
			message:       `Invalid, expired or revoked API key. Create a new one in ` + settingsPage + ` and send it in the Authorization header as "Bearer <key>".`,
			challenge:     `Bearer realm="beszel", error="invalid_token", error_description="Invalid, expired or revoked API key. Create a new one in ` + settingsPage + ` and send it in the Authorization header as 'Bearer <key>'."`,
		},
		{
			name:          "expired",
			authorization: "Bearer " + expiredToken,
			message:       `Invalid, expired or revoked API key. Create a new one in ` + settingsPage + ` and send it in the Authorization header as "Bearer <key>".`,
			challenge:     `Bearer realm="beszel", error="invalid_token", error_description="Invalid, expired or revoked API key. Create a new one in ` + settingsPage + ` and send it in the Authorization header as 'Bearer <key>'."`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// every registered method is guarded, not only POST
			for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
				req, err := http.NewRequest(method, server.URL+"/mcp",
					strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json, text/event-stream")
				if tt.authorization != "" {
					req.Header.Set("Authorization", tt.authorization)
				}
				res, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()
				require.Equal(t, http.StatusUnauthorized, res.StatusCode, method)
				require.Equal(t, tt.challenge, res.Header.Get("WWW-Authenticate"), method)
				var body struct {
					Status  int    `json:"status"`
					Message string `json:"message"`
				}
				require.NoError(t, json.NewDecoder(res.Body).Decode(&body), method)
				require.Equal(t, http.StatusUnauthorized, body.Status, method)
				require.Equal(t, tt.message, body.Message, method)
			}
		})
	}
}

func TestMcpUnauthorizedWithoutAppURL(t *testing.T) {
	hub, _, _, server, _ := setupHub(t)
	hub.Settings().Meta.AppURL = ""
	res, err := http.Post(server.URL+"/mcp", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	var body struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, `Missing API key. Create one in Settings > API Keys and send it in the Authorization header as "Bearer <key>".`, body.Message)
}

// TestMcpStaticApiKey verifies that API keys created on the settings page
// (static, non-refreshable user tokens) authenticate MCP requests.
func TestMcpStaticApiKey(t *testing.T) {
	_, user, _, server, _ := setupHub(t)
	apiKey, err := user.Record.NewStaticAuthToken(time.Hour)
	require.NoError(t, err)
	session := mcpClient(t, server.URL, "Bearer "+apiKey)
	result := callTool(t, session, "list_systems", nil)
	require.Len(t, result["systems"], 2)
}

// mcpInitialize sends a raw initialize request to the /mcp endpoint with the
// given Host header (empty keeps the server address).
func mcpInitialize(t *testing.T, serverURL string, token string, host string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, serverURL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if host != "" {
		req.Host = host
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestMcpReverseProxyHost(t *testing.T) {
	_, user, _, server, _ := setupHub(t)
	// a reverse proxy on the same host connects over loopback but forwards
	// the public Host header; this must not be rejected as DNS rebinding
	res := mcpInitialize(t, server.URL, user.Token, "beszel.example.com")
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestMcpStateless(t *testing.T) {
	_, user, _, server, _ := setupHub(t)
	// no session is created, so idle sessions can't accumulate in memory
	res := mcpInitialize(t, server.URL, user.Token, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Empty(t, res.Header.Get("Mcp-Session-Id"))
	// GET (standalone SSE stream) is answered by the MCP handler with 405
	// rather than falling through to the web UI
	req, err := http.NewRequest(http.MethodGet, server.URL+"/mcp", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", user.Token)
	req.Header.Set("Accept", "text/event-stream")
	getRes, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer getRes.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, getRes.StatusCode)
}

func TestMcpListSystems(t *testing.T) {
	_, user, _, server, _ := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "list_systems", nil)
	systems := data["systems"].([]any)
	names := []string{}
	for _, system := range systems {
		names = append(names, system.(map[string]any)["name"].(string))
	}
	require.ElementsMatch(t, []string{"user-system-1", "user-system-2"}, names)
	first := systems[0].(map[string]any)
	require.Equal(t, float64(42), first["cpu_pct"])
	require.Equal(t, float64(51), first["memory_pct"])
	require.Equal(t, "up", first["status"])
	// legacy Info.Bandwidth (MB/s) is converted to bytes/sec
	require.Equal(t, float64(1572864), first["bandwidth_bps"])
	require.NotContains(t, data, "truncated")
}

func TestMcpGetSystem(t *testing.T) {
	_, user, _, server, userSystem := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "get_system", map[string]any{"system": "user-system-1"})
	require.Equal(t, "user-system-1", data["name"])
	serverInfo := data["server"].(map[string]any)
	require.Equal(t, "Test CPU 9000", serverInfo["cpu_model"])
	require.Equal(t, float64(8), serverInfo["cores"])
	require.Equal(t, "x86_64", serverInfo["arch"])
	require.Equal(t, float64(8589934592), serverInfo["memory_bytes"])

	// look up by id works
	data = callTool(t, session, "get_system", map[string]any{"system": userSystem.Id})
	require.Equal(t, "user-system-1", data["name"])

	// foreign system (by name or id) is not visible
	errText := callToolError(t, session, "get_system", map[string]any{"system": "other-system"})
	require.Contains(t, errText, "other-system")
	require.Contains(t, errText, "not found or not visible")
}

func TestMcpGetSystemStats(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	// modern (bytes/s, with per-core usage), legacy network (MB/s) and legacy
	// disk (MB/s) formats
	createStatsRecord(t, hub, userSystem.Id, map[string]any{"cpu": 80, "mp": 40, "dp": 20, "b": [2]uint64{1000, 2000}, "cpus": []uint16{70, 90}}, 30)
	createStatsRecord(t, hub, userSystem.Id, map[string]any{"cpu": 40, "mp": 60, "dp": 40, "ns": 1, "nr": 2}, 20)
	createStatsRecord(t, hub, userSystem.Id, map[string]any{"cpu": 10, "mp": 10, "dp": 10, "dr": 2, "dw": 1}, 10)

	// a 1 hour lookback defaults to the 1m resolution
	data := callTool(t, session, "get_system_stats", map[string]any{"system": "user-system-1", "hours": 1})
	require.Equal(t, "1m", data["period"])
	samples := data["samples"].([]any)
	require.Len(t, samples, 3)
	require.Equal(t, float64(1000), samples[0].(map[string]any)["net_sent_bps"])
	require.Equal(t, float64(1048576), samples[1].(map[string]any)["net_sent_bps"])
	// legacy disk MB/s fields are converted to bytes/sec
	require.Equal(t, float64(2097152), samples[2].(map[string]any)["disk_read_bps"])
	require.Equal(t, float64(1048576), samples[2].(map[string]any)["disk_write_bps"])
	summary := data["summary"].(map[string]any)
	require.InDelta(t, 43.3333, summary["avg_cpu_pct"], 0.001)
	require.Equal(t, float64(80), summary["max_cpu_pct"])
	require.InDelta(t, 36.6667, summary["avg_memory_pct"], 0.001)
	// summary applies the same legacy conversions (sent, recv order)
	maxBandwidth := summary["max_bandwidth_bps"].([]any)
	require.Equal(t, float64(1048576), maxBandwidth[0])
	require.Equal(t, float64(2097152), maxBandwidth[1])
	maxDiskIO := summary["max_disk_io_bps"].([]any)
	require.Equal(t, float64(2097152), maxDiskIO[0])
	require.Equal(t, float64(1048576), maxDiskIO[1])

	// invalid period
	errText := callToolError(t, session, "get_system_stats", map[string]any{"system": "user-system-1", "period": "5s"})
	require.Contains(t, errText, "invalid period")
}

func TestMcpListContainers(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	_, err := beszelTests.CreateRecord(hub, "container_stats", map[string]any{
		"system": userSystem.Id,
		"type":   "1m",
		"stats":  []map[string]any{{"n": "web", "c": 12.5, "m": 256, "b": [2]uint64{500, 1500}}},
	})
	require.NoError(t, err)

	data := callTool(t, session, "list_containers", map[string]any{"system": "user-system-1"})
	containers := data["containers"].([]any)
	require.Len(t, containers, 1)
	container := containers[0].(map[string]any)
	require.Equal(t, "web", container["name"])
	require.Equal(t, float64(12.5), container["cpu_pct"])
	require.Equal(t, float64(256), container["mem_mb"])
	require.Equal(t, []any{float64(500), float64(1500)}, container["bandwidth_bps"])
}

func TestMcpListAlerts(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	_, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name": "CPU", "user": user.Record.Id, "system": userSystem.Id, "value": 90,
	})
	require.NoError(t, err)

	// regular user sees only their own alerts
	data := callTool(t, session, "list_alerts", nil)
	alerts := data["alerts"].([]any)
	require.Len(t, alerts, 1)
	alert := alerts[0].(map[string]any)
	require.Equal(t, "CPU", alert["name"])
	require.Equal(t, float64(90), alert["value"])
	// unset JSON state is omitted, not rendered as the literal string "null"
	require.NotContains(t, alert, "state")
	// system names are resolved at the list level
	require.Equal(t, map[string]any{userSystem.Id: "user-system-1"}, data["systems"])

	// superuser sees all alerts
	superuser, err := beszelTests.CreateSuperuser(hub, "mcp-superuser@example.com", "password")
	require.NoError(t, err)
	superToken, err := superuser.NewAuthToken()
	require.NoError(t, err)
	superSession := mcpClient(t, server.URL, superToken)
	data = callTool(t, superSession, "list_alerts", nil)
	alerts = data["alerts"].([]any)
	require.Len(t, alerts, 1)
}

func TestMcpToolsList(t *testing.T) {
	_, user, _, server, _ := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	listResult, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t, []string{
		"list_systems", "get_system", "get_system_stats", "list_containers", "list_alerts",
		"list_alert_events", "list_services", "get_disk_health", "list_network_monitors",
	}, names)
}

func TestMcpShareAllSystems(t *testing.T) {
	_, user, _, server, _ := setupHubWithShareAll(t, "true")
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "list_systems", nil)
	require.Equal(t, float64(3), data["count"])
	// another user's system is readable when SHARE_ALL_SYSTEMS is enabled
	data = callTool(t, session, "get_system", map[string]any{"system": "other-system"})
	require.Equal(t, "other-system", data["name"])
}

func TestMcpAmbiguousSystemName(t *testing.T) {
	hub, user, _, server, _ := setupHub(t)
	// a second system with the same name owned by the same user
	createSystem(t, hub, "user-system-1", user.Record.Id)
	session := mcpClient(t, server.URL, user.Token)
	errText := callToolError(t, session, "get_system", map[string]any{"system": "user-system-1"})
	require.Contains(t, errText, "ambiguous")
}

func TestMcpBearerPrefix(t *testing.T) {
	_, user, _, server, _ := setupHub(t)
	// the Bearer prefix, as sent by most real MCP clients, is accepted
	session := mcpClient(t, server.URL, "Bearer "+user.Token)
	data := callTool(t, session, "list_systems", nil)
	require.Equal(t, float64(2), data["count"])
}

func TestMcpStatsHoursClamp(t *testing.T) {
	_, user, _, server, _ := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "get_system_stats", map[string]any{"system": "user-system-1", "hours": 1000})
	// lookback is clamped to 720 hours (480m retention)
	require.Equal(t, "480m", data["period"])
	from, err := time.Parse(time.RFC3339, data["from"].(string))
	require.NoError(t, err)
	require.WithinDuration(t, from, time.Now().UTC().Add(-720*time.Hour), 5*time.Minute)
}

func TestMcpStatsDefaultPeriod(t *testing.T) {
	_, user, _, server, _ := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	// without a period, the finest resolution covering the lookback is used
	for _, tc := range []struct {
		hours  any
		period string
	}{
		{nil, "20m"}, // default 24 hours
		{0.5, "1m"},
		{1, "1m"},
		{2, "10m"},
		{12, "10m"},
		{24, "20m"},
		{48, "120m"},
		{168, "120m"},
		{169, "480m"},
		{720, "480m"},
	} {
		args := map[string]any{"system": "user-system-1"}
		if tc.hours != nil {
			args["hours"] = tc.hours
		}
		data := callTool(t, session, "get_system_stats", args)
		require.Equal(t, tc.period, data["period"], "hours=%v", tc.hours)
		// no records: samples is an empty array, not null
		require.Equal(t, []any{}, data["samples"], "hours=%v", tc.hours)
	}
}

func TestMcpStatsExplicitPeriodRetention(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	createStatsRecord(t, hub, userSystem.Id, map[string]any{"cpu": 50}, 10)
	// an explicit period is honored, but the reported range is shortened to
	// its retention instead of claiming to cover the requested hours
	data := callTool(t, session, "get_system_stats", map[string]any{"system": "user-system-1", "period": "1m", "hours": 24})
	require.Equal(t, "1m", data["period"])
	require.Len(t, data["samples"].([]any), 1)
	require.Contains(t, data["note"], "only kept for 1 hour,")
	from, err := time.Parse(time.RFC3339, data["from"].(string))
	require.NoError(t, err)
	require.WithinDuration(t, from, time.Now().UTC().Add(-time.Hour), 5*time.Minute)
}

func TestMcpTruncatedSystems(t *testing.T) {
	hub, user, _, server, _ := setupHub(t)
	// fill beyond the 200 record cap
	for i := range 201 {
		_, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
			"name":  fmt.Sprintf("bulk-system-%03d", i),
			"host":  "127.0.0.1",
			"users": []string{user.Record.Id},
		})
		require.NoError(t, err)
	}
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "list_systems", nil)
	// the 200 record cap is reached and flagged
	require.Equal(t, float64(200), data["count"])
	require.Equal(t, true, data["truncated"])
}

func TestMcpGetSystemUpdatesAndCurrent(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	// pending package updates ("pu": [total, security]), battery ("bat":
	// [percent, charge state]) and connected Wi-Fi ("wf") come from system info
	reloadSystem := hubFindSystem(t, hub, userSystem.Id)
	reloadSystem.Set("info", map[string]any{
		"cpu": 42,
		"dt":  61.5,
		"pu":  [2]uint16{12, 2},
		"bat": [2]int{80, 3}, // plain ints: json/v2 encodes [2]uint8 as base64
		"wf": map[string]any{
			"wlan0": map[string]any{"s": "home", "r": -52},
			"wlan1": map[string]any{"s": "guest"}, // associated, no RSSI
		},
	})
	require.NoError(t, hub.Save(reloadSystem))
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "get_system", map[string]any{"system": "user-system-1"})
	require.Equal(t, map[string]any{"total": float64(12), "security": float64(2)}, data["updates"])
	require.Equal(t, map[string]any{"percent": float64(80), "state": "charging"}, data["battery"])
	wifi := map[string]any{
		"wlan0": map[string]any{"ssid": "home", "signal_dbm": float64(-52)},
		"wlan1": map[string]any{"ssid": "guest"},
	}
	require.Equal(t, wifi, data["wifi"])
	// list_systems carries the same summary fields
	list := callTool(t, session, "list_systems", nil)
	for _, entry := range list["systems"].([]any) {
		if entry.(map[string]any)["name"] == "user-system-1" {
			require.Equal(t, wifi, entry.(map[string]any)["wifi"])
			require.Equal(t, 61.5, entry.(map[string]any)["temp_c"])
			require.Equal(t, map[string]any{"percent": float64(80), "state": "charging"}, entry.(map[string]any)["battery"])
		}
	}

	// detailed current metrics come from the latest 1m system_stats record
	createStatsRecord(t, hub, userSystem.Id, map[string]any{
		"cpu":  55,
		"t":    map[string]float64{"cpu": 45.5, "nvme": 38},
		"g":    map[string]any{"nvidia0": map[string]any{"n": "RTX", "u": 30, "mu": 1024, "mt": 8192}},
		"swap": [2]uint64{1073741824, 536870912},
		"f":    map[string]uint16{"fan1": 1200},
		"efs":  map[string]any{"mnt/data": map[string]any{"d": 100, "du": 20, "rb": 4096, "wb": 2048}},
		"bat":  [2]uint8{80, 1},
		// per-core usage sent by current agents (a byte slice that json/v2
		// rejects unless it unmarshals numeric arrays itself)
		"cpus": []uint16{50, 60},
	}, 1)
	data = callTool(t, session, "get_system", map[string]any{"system": "user-system-1"})
	current := data["current"].(map[string]any)
	require.Equal(t, map[string]any{"cpu": float64(45.5), "nvme": float64(38)}, current["temperatures"])
	gpus := current["gpus"].(map[string]any)["nvidia0"].(map[string]any)
	require.Equal(t, float64(30), gpus["u"])
	require.Equal(t, []any{float64(1073741824), float64(536870912)}, current["swap_bytes"])
	require.Equal(t, map[string]any{"fan1": float64(1200)}, current["fans"])
	filesystems := current["filesystems"].(map[string]any)["mnt/data"].(map[string]any)
	require.Equal(t, float64(4096), filesystems["read_bps"])
	require.Equal(t, float64(2048), filesystems["write_bps"])
	// battery is reported once, in the summary
	require.NotContains(t, current, "battery")
}

// hubFindSystem reloads a system record for updates (create hooks reset fields).
func hubFindSystem(t *testing.T, hub *beszelTests.TestHub, systemId string) *core.Record {
	t.Helper()
	system, err := hub.FindRecordById("systems", systemId)
	require.NoError(t, err)
	return system
}

func TestMcpListServices(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	_, err := beszelTests.CreateRecord(hub, "systemd_services", map[string]any{
		"system": userSystem.Id, "name": "nginx.service", "state": 2, "sub": 3,
		"cpu": 12.5, "cpuPeak": 30, "memory": 456789, "memPeak": 500000,
		"updated": time.Now().UTC().UnixMilli(),
	})
	require.NoError(t, err)
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "list_services", map[string]any{"system": "user-system-1"})
	services := data["services"].([]any)
	require.Len(t, services, 1)
	service := services[0].(map[string]any)
	require.Equal(t, "nginx.service", service["name"])
	require.Equal(t, "failed", service["state"])
	require.Equal(t, float64(30), service["cpu_peak_pct"])
	require.Equal(t, float64(456789), service["mem_bytes"])
}

func TestMcpDiskHealth(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	_, err := beszelTests.CreateRecord(hub, "smart_devices", map[string]any{
		"system": userSystem.Id, "name": "nvme0n1", "model": "Samsung 990 Pro", "state": "PASSED",
		"type": "ssd", "serial": "S6ZNF0", "firmware": "2B2QGXA7", "capacity": 1024110208,
		"temp": 45, "hours": 12345, "cycles": 100,
	})
	require.NoError(t, err)
	_, err = beszelTests.CreateRecord(hub, "zfs_pools", map[string]any{
		"system": userSystem.Id, "name": "tank", "display_name": "Tank", "health": "ONLINE",
		"size": 4096, "alloc": 1024, "free": 3072, "updated": time.Now().UTC(),
	})
	require.NoError(t, err)
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "get_disk_health", map[string]any{"system": "user-system-1"})
	smart := data["smart_devices"].([]any)
	require.Len(t, smart, 1)
	device := smart[0].(map[string]any)
	require.Equal(t, "nvme0n1", device["name"])
	require.Equal(t, "PASSED", device["state"])
	require.Equal(t, float64(45), device["temp_c"])
	require.Equal(t, float64(12345), device["power_on_hours"])
	pools := data["zfs_pools"].([]any)
	require.Len(t, pools, 1)
	pool := pools[0].(map[string]any)
	require.Equal(t, "tank", pool["name"])
	require.Equal(t, "ONLINE", pool["health"])
	require.Equal(t, float64(1024), pool["alloc_bytes"])
}

func TestMcpNetworkMonitors(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	_, err := beszelTests.CreateRecord(hub, "network_monitors", map[string]any{
		"system": userSystem.Id, "target": "1.1.1.1", "protocol": "icmp", "interval": 60,
		// response times are stored in microseconds
		"enabled": true, "res": 12500, "resAvg1h": 11000, "resMin1h": 9000, "resMax1h": 40250, "loss1h": 0.5,
	})
	require.NoError(t, err)
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "list_network_monitors", map[string]any{"system": "user-system-1"})
	monitors := data["monitors"].([]any)
	require.Len(t, monitors, 1)
	monitor := monitors[0].(map[string]any)
	require.Equal(t, "1.1.1.1", monitor["target"])
	require.Equal(t, "icmp", monitor["protocol"])
	require.Equal(t, true, monitor["enabled"])
	require.Equal(t, float64(12.5), monitor["latency_ms"])
	require.Equal(t, float64(11), monitor["latency_avg_1h_ms"])
	require.Equal(t, float64(40.25), monitor["latency_max_1h_ms"])
	require.Equal(t, float64(0.5), monitor["loss_pct_1h"])
}

func TestMcpListAlertEvents(t *testing.T) {
	hub, user, other, server, userSystem := setupHub(t)
	_, err := beszelTests.CreateRecord(hub, "alerts_history", map[string]any{
		"user": user.Record.Id, "system": userSystem.Id, "alert_id": "alert-1",
		"name": "CPU", "value": 95.5,
	})
	require.NoError(t, err)
	_, err = beszelTests.CreateRecord(hub, "alerts_history", map[string]any{
		"user": other.Record.Id, "system": getSystemIdByName(t, hub, "other-system"), "alert_id": "alert-2",
		"name": "Memory", "value": 91,
	})
	require.NoError(t, err)
	session := mcpClient(t, server.URL, user.Token)
	data := callTool(t, session, "list_alert_events", nil)
	// regular user sees only their own alert events
	events := data["events"].([]any)
	require.Len(t, events, 1)
	event := events[0].(map[string]any)
	require.Equal(t, "CPU", event["name"])
	require.Equal(t, float64(95.5), event["value"])
	// resolved_at stays unset while the event is ongoing
	require.NotContains(t, event, "resolved_at")
	// system names resolved at the list level
	require.Equal(t, map[string]any{userSystem.Id: "user-system-1"}, data["systems"])

	// superuser sees all events
	superuser, err := beszelTests.CreateSuperuser(hub, "mcp-events-super@example.com", "password")
	require.NoError(t, err)
	superToken, err := superuser.NewAuthToken()
	require.NoError(t, err)
	superSession := mcpClient(t, server.URL, superToken)
	data = callTool(t, superSession, "list_alert_events", nil)
	require.Len(t, data["events"].([]any), 2)
}

// getSystemIdByName returns a system's id by exact name.
func getSystemIdByName(t *testing.T, hub *beszelTests.TestHub, name string) string {
	t.Helper()
	system, err := hub.FindFirstRecordByFilter("systems", "name = {:name}", dbx.Params{"name": name})
	require.NoError(t, err)
	return system.Id
}

func TestMcpStatsUndecodableRecords(t *testing.T) {
	hub, user, _, server, userSystem := setupHub(t)
	session := mcpClient(t, server.URL, user.Token)
	createStatsRecord(t, hub, userSystem.Id, map[string]any{"cpu": 20}, 20)
	// valid JSON that doesn't match the stats struct is skipped with a note
	// instead of silently shrinking the result
	createStatsRecord(t, hub, userSystem.Id, map[string]any{"cpu": "not a number"}, 10)
	data := callTool(t, session, "get_system_stats", map[string]any{"system": "user-system-1", "hours": 1})
	require.Len(t, data["samples"].([]any), 1)
	require.Contains(t, data["note"], "1 of 2 records could not be decoded")
}
