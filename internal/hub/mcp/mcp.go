// Package mcp exposes the hub's monitoring data to AI agents over the Model
// Context Protocol (MCP).
//
// The server is mounted on the /mcp route (streamable HTTP transport) when the
// BESZEL_HUB_MCP env var is set to "true". It is protected by the standard
// PocketBase auth middleware, so MCP clients must send a valid auth token in
// the Authorization header. Only read-only tools are exposed.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/henrygd/beszel"
	"github.com/henrygd/beszel/internal/hub/utils"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterRoutes registers the /mcp route if MCP is enabled with the
// BESZEL_HUB_MCP=true env var.
func RegisterRoutes(app core.App, se *core.ServeEvent) error {
	if enabled, _ := utils.GetEnv("MCP"); enabled != "true" {
		return nil
	}
	server := newServer(app)
	handler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{
		// all tools are read-only and authorize every request, so no session
		// state is needed (stateful sessions are never freed if clients don't
		// send DELETE)
		Stateless: true,
		// requests are authenticated with a token header rather than cookies,
		// so DNS rebinding is not a concern, and the protection rejects hubs
		// behind a reverse proxy on the same host (loopback address with a
		// public Host header)
		DisableLocalhostProtection: true,
	})
	// authorization is handled by PocketBase (401 for missing/invalid tokens)
	// so MCP clients only need to pass a valid auth token in the Authorization
	// header, e.g. Authorization: Bearer <token>
	// (the stateless transport uses POST for messages and answers GET and
	// DELETE with 405 so clients know there is no SSE stream or session; each
	// method is registered separately as a method wildcard route would
	// conflict with the static file catch-all route, which would otherwise
	// serve the web UI for GET /mcp)
	mcpHandler := func(e *core.RequestEvent) error {
		// pass the auth record resolved by PocketBase to the tool handlers
		// (stateless sessions run tool calls with the request context)
		ctx := context.WithValue(e.Request.Context(), authContextKey{}, e.Auth)
		handler.ServeHTTP(e.Response, e.Request.WithContext(ctx))
		return nil
	}
	for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		se.Router.Route(method, "/mcp", mcpHandler).Bind(apis.RequireAuth())
	}
	return nil
}

// newServer creates an MCP server with the hub's read-only tools.
func newServer(app core.App) *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "beszel",
		Version: beszel.Version,
	}, &mcpsdk.ServerOptions{
		Instructions: "Beszel is a lightweight server monitoring hub. " +
			"Use list_systems to see monitored systems and their current usage (including battery and Wi-Fi signal), " +
			"get_system for hardware details and current sensors, get_system_stats " +
			"for historical metrics, list_containers for docker container usage, " +
			"list_services for systemd services, get_disk_health for SMART and ZFS " +
			"health, list_network_monitors for endpoint monitoring, and list_alerts / " +
			"list_alert_events for alert configurations and triggered events. Data " +
			"access matches the permissions of the authenticated hub user.",
	})
	registerTools(app, server)
	return server
}

// mcphub bundles an app with the MCP server so tool handlers can query data.
type mcphub struct {
	app core.App
}

// notVisibleMsg is shared for missing and inaccessible systems so callers
// cannot distinguish between the two.
const notVisibleMsg = "not found or not visible to this user"

// authContextKey is the request context key of the auth record.
type authContextKey struct{}

// authRecord returns the authenticated record (regular user or superuser)
// that PocketBase resolved from the Authorization header of the HTTP request
// carrying the tool call. Access is enforced by the auth middleware on the
// /mcp route, but tools use the record to scope queries to visible systems.
func (h *mcphub) authRecord(ctx context.Context) (*core.Record, error) {
	record, _ := ctx.Value(authContextKey{}).(*core.Record)
	if record == nil {
		return nil, errors.New("missing authenticated user; add a valid PocketBase user token to the Authorization header of your MCP client")
	}
	return record, nil
}

// shareAllSystems reports whether any authenticated user can see all systems
// (mirrors the SHARE_ALL_SYSTEMS env var behavior used by collection rules).
func shareAllSystems() bool {
	value, _ := utils.GetEnv("SHARE_ALL_SYSTEMS")
	return value == "true"
}

// resolveSystem finds a system by name or id, restricted to systems visible
// to the auth record. An exact id match takes precedence over name matches; a
// name matching multiple visible systems is rejected instead of being resolved
// to an arbitrary match.
func (h *mcphub) resolveSystem(auth *core.Record, system string) (*core.Record, error) {
	notFound := fmt.Errorf("system %q "+notVisibleMsg, system)
	records, err := h.app.FindRecordsByFilter("systems", "name = {:s} || id = {:s}", "", maxListRecords, 0, dbx.Params{"s": system})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, notFound
	}
	// exact id match wins over name matches
	for _, record := range records {
		if record.Id == system {
			return h.visibleSystem(auth, record, notFound)
		}
	}
	// name matches: only consider systems visible to the user
	visible := make([]*core.Record, 0, len(records))
	for _, record := range records {
		if _, err := h.visibleSystem(auth, record, notFound); err == nil {
			visible = append(visible, record)
		}
	}
	switch {
	case len(visible) == 0:
		return nil, notFound
	case len(visible) > 1:
		return nil, fmt.Errorf("system name %q is ambiguous (%d visible systems match); use the system id instead", system, len(visible))
	}
	return visible[0], nil
}

// visibleSystem returns the system if it is visible to the auth record.
func (h *mcphub) visibleSystem(auth *core.Record, record *core.Record, notFound error) (*core.Record, error) {
	if auth.IsSuperuser() || shareAllSystems() || slices.Contains(record.GetStringSlice("users"), auth.Id) {
		return record, nil
	}
	return nil, notFound
}
