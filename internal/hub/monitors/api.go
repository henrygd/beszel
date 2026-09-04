package monitors

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// redactedSecret masks secret values in API responses.
const redactedSecret = "••••••"

// secretConfigKeys are config keys never returned in clear text.
var secretConfigKeys = map[string]bool{"password": true, "token": true}

// MonitorAPI exposes monitor CRUD, history, testing and summary endpoints.
// The scheduler is wired in Task 10; check runs inline here for test/manual runs.
type MonitorAPI struct {
	app core.App
}

// testLimiter holds sliding-window timestamps for the manual-test endpoint.
// It is shared per app (not per MonitorAPI instance): production mounts one
// instance, but tests mount one per scenario on the same app, and the
// rate limit must apply across them.
type testLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

var testLimiters sync.Map // core.App -> *testLimiter

func limiterFor(app core.App) *testLimiter {
	if v, ok := testLimiters.Load(app); ok {
		return v.(*testLimiter)
	}
	l := &testLimiter{hits: make(map[string][]time.Time)}
	actual, _ := testLimiters.LoadOrStore(app, l)
	return actual.(*testLimiter)
}

// RegisterRoutes mounts the monitors API under /api/beszel/monitors on the
// given serve event's router. It must be called synchronously from the
// parent OnServe handler (registerApiRoutes): PocketBase snapshots OnServe
// handlers when the event triggers, so binding a nested OnServe hook here
// would never fire in production.
func RegisterRoutes(se *core.ServeEvent) {
	api := &MonitorAPI{app: se.App}
	g := se.Router.Group("/api/beszel/monitors")
	g.Bind(apis.RequireAuth())
	g.GET("", api.list)
	g.POST("", api.create)
	g.GET("/summary", api.summary)
	g.GET("/{id}", api.get)
	g.PATCH("/{id}", api.update)
	g.DELETE("/{id}", api.delete)
	g.GET("/{id}/checks", api.checks)
	g.POST("/{id}/test", api.test)
}

type monitorInput struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Target      string         `json:"target"`
	Interval    *int           `json:"interval"`
	Timeout     *int           `json:"timeout"`
	MaxRetries  *int           `json:"max_retries"`
	UpsideDown  *bool          `json:"upside_down"`
	Paused      *bool          `json:"paused"`
	Notify      *bool          `json:"notify"`
	ResendAfter *int           `json:"resend_after"`
	Users       []string       `json:"users"`
	Config      map[string]any `json:"config"`
}

func intOr(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// validateInput applies defaults and returns field-typed errors. It
// validates the API-level surface (types, schemes, ports, enums, bounds);
// fine-grained config semantics stay with the checkers, which report typed
// Down results at check time.
func validateInput(in *monitorInput) error {
	m := Monitor{
		Name: in.Name, Type: MonitorType(in.Type), Target: strings.TrimSpace(in.Target),
		IntervalSeconds: intOr(in.Interval, 60), TimeoutSeconds: intOr(in.Timeout, 10),
		MaxRetries: intOr(in.MaxRetries, 2), UpsideDown: boolOr(in.UpsideDown, false),
	}
	if in.Name == "" {
		return fmt.Errorf("name is required")
	}
	if err := m.Validate(); err != nil {
		return err
	}
	if ra := intOr(in.ResendAfter, 0); ra < 0 || ra > 1440 {
		return fmt.Errorf("resend_after must be 0..1440 minutes")
	}
	switch m.Type {
	case TypeHTTP, TypeKeyword:
		u, err := parseMonitorURL(m.Target)
		if err != nil {
			return fmt.Errorf("invalid http target: %v", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("http target must use http(s) scheme")
		}
		if u.Hostname() == "" {
			return fmt.Errorf("http target must have a host")
		}
		if in.Config != nil {
			if method, ok := in.Config["method"]; ok {
				s, _ := method.(string)
				if !allowedHTTPMethods[strings.ToUpper(s)] {
					return fmt.Errorf("invalid method %q", s)
				}
			}
		}
	case TypeTLS:
		// Same parser as the checker: URL (https://host[:port]/...) or
		// host[:port], default 443. Sharing it guarantees API/checker parity.
		host, port, err := parseTLSTarget(m.Target)
		if err != nil {
			return fmt.Errorf("invalid tls target: %v", err)
		}
		if host == "" || port <= 0 || port > 65535 {
			return fmt.Errorf("tls target must be host[:port]")
		}
	case TypeDNS:
		if in.Config != nil {
			if qtype, ok := in.Config["qtype"]; ok {
				s, _ := qtype.(string)
				if _, ok := allowedDNSQTypes[strings.ToUpper(s)]; !ok {
					return fmt.Errorf("invalid qtype %q", s)
				}
			}
			if proto, ok := in.Config["protocol"]; ok {
				s, _ := proto.(string)
				if s != "udp" && s != "tcp" {
					return fmt.Errorf("invalid protocol %q: must be udp or tcp", s)
				}
			}
			if resolver, ok := in.Config["resolver"]; ok {
				if s, _ := resolver.(string); s != "" {
					host, _, err := ParsePortOrDefault(s, 53)
					if err != nil {
						return fmt.Errorf("invalid resolver: %v", err)
					}
					if net.ParseIP(stripBrackets(host)) == nil {
						return fmt.Errorf("resolver must be an IP address")
					}
				}
			}
		}
	case TypePing:
		if strings.TrimSpace(m.Target) == "" {
			return fmt.Errorf("ping target must not be empty")
		}
	}
	return nil
}

func redactConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if secretConfigKeys[strings.ToLower(k)] {
			out[k] = redactedSecret
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			out[k] = redactConfig(nested)
			continue
		}
		out[k] = v
	}
	return out
}

func monitorToResponse(rec *core.Record) map[string]any {
	cfg := map[string]any{}
	_ = rec.UnmarshalJSONField("config", &cfg)
	return map[string]any{
		"id": rec.Id, "name": rec.GetString("name"), "type": rec.GetString("type"),
		"target": rec.GetString("target"), "interval": rec.GetFloat("interval"),
		"timeout": rec.GetFloat("timeout"), "max_retries": rec.GetFloat("max_retries"),
		"upside_down": rec.GetBool("upside_down"), "paused": rec.GetBool("paused"),
		"notify": rec.GetBool("notify"), "resend_after": rec.GetFloat("resend_after"),
		"users": rec.GetStringSlice("users"), "config": redactConfig(cfg),
		"status": rec.GetString("status"), "last_check": rec.GetDateTime("last_check"),
		"last_latency_ms": rec.GetFloat("last_latency_ms"), "uptime_24h": rec.GetFloat("uptime_24h"),
		"cert_days": rec.GetFloat("cert_days"),
		"created":   rec.GetDateTime("created"), "updated": rec.GetDateTime("updated"),
	}
}

// monitorHasUser mirrors alerts.userHasSystem plus SHARE_ALL_SYSTEMS.
func monitorHasUser(app core.App, userID string, rec *core.Record) bool {
	if shareAllSystems(app) {
		return true
	}
	for _, u := range rec.GetStringSlice("users") {
		if u == userID {
			return true
		}
	}
	return false
}

func shareAllSystems(_ core.App) bool {
	return os.Getenv("SHARE_ALL_SYSTEMS") == "true"
}

// filterMonitorRecords applies membership and paused filters in Go instead
// of SQL: relation membership needs PocketBase filter parsing with joins,
// which FindAllRecords with raw dbx expressions cannot express. Monitor
// counts per hub stay small (Task 13 load test), so this is safe.
func filterMonitorRecords(recs []*core.Record, userID, paused string) []*core.Record {
	// Fresh slice (never recs[:0]): aliasing the input would corrupt
	// callers that reuse the slice across calls.
	out := make([]*core.Record, 0, len(recs))
	for _, rec := range recs {
		member := shareAllSystems(nil)
		if !member {
			for _, u := range rec.GetStringSlice("users") {
				if u == userID {
					member = true
					break
				}
			}
		}
		if !member {
			continue
		}
		if paused != "" {
			want := paused == "true" || paused == "1"
			if rec.GetBool("paused") != want {
				continue
			}
		}
		out = append(out, rec)
	}
	return out
}

// sortMonitorsByName orders monitors for the list endpoint (spec §7).
func sortMonitorsByName(recs []*core.Record) {
	sort.Slice(recs, func(i, j int) bool { return recs[i].GetString("name") < recs[j].GetString("name") })
}

func (a *MonitorAPI) findMonitor(e *core.RequestEvent, id string) (*core.Record, error) {
	rec, err := a.app.FindRecordById("monitors", id)
	if err != nil {
		return nil, e.NotFoundError("monitor not found", err)
	}
	if !monitorHasUser(a.app, e.Auth.Id, rec) {
		return nil, e.NotFoundError("monitor not found", nil)
	}
	return rec, nil
}

func (a *MonitorAPI) list(e *core.RequestEvent) error {
	recs, err := a.app.FindAllRecords("monitors")
	if err != nil {
		return e.InternalServerError("", err)
	}
	recs = filterMonitorRecords(recs, e.Auth.Id, e.Request.URL.Query().Get("paused"))
	sortMonitorsByName(recs)
	out := make([]map[string]any, 0, len(recs))
	for _, rec := range recs {
		out = append(out, monitorToResponse(rec))
	}
	return e.JSON(http.StatusOK, out)
}

func (a *MonitorAPI) create(e *core.RequestEvent) error {
	if e.Auth.GetString("role") == "readonly" {
		return e.ForbiddenError("readonly users cannot create monitors", nil)
	}
	var in monitorInput
	if err := e.BindBody(&in); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if len(in.Users) == 0 {
		in.Users = []string{e.Auth.Id}
	}
	if err := validateInput(&in); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	col, err := a.app.FindCachedCollectionByNameOrId("monitors")
	if err != nil {
		return e.InternalServerError("", err)
	}
	rec := core.NewRecord(col)
	rec.Set("name", in.Name)
	rec.Set("type", in.Type)
	rec.Set("target", strings.TrimSpace(in.Target))
	rec.Set("interval", intOr(in.Interval, 60))
	rec.Set("timeout", intOr(in.Timeout, 10))
	rec.Set("max_retries", intOr(in.MaxRetries, 2))
	rec.Set("upside_down", in.UpsideDown)
	rec.Set("paused", in.Paused)
	rec.Set("notify", boolOr(in.Notify, true))
	rec.Set("resend_after", intOr(in.ResendAfter, 0))
	rec.Set("users", in.Users)
	if in.Config == nil {
		in.Config = map[string]any{}
	}
	rec.Set("config", in.Config)
	rec.Set("status", "pending")
	if err := a.app.Save(rec); err != nil {
		return e.BadRequestError("failed to save monitor", err)
	}
	return e.JSON(http.StatusCreated, monitorToResponse(rec))
}

func (a *MonitorAPI) get(e *core.RequestEvent) error {
	rec, err := a.findMonitor(e, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	return e.JSON(http.StatusOK, monitorToResponse(rec))
}

func (a *MonitorAPI) update(e *core.RequestEvent) error {
	if e.Auth.GetString("role") == "readonly" {
		return e.ForbiddenError("readonly users cannot update monitors", nil)
	}
	rec, err := a.findMonitor(e, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	var in monitorInput
	if err := e.BindBody(&in); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	// Merge with existing values, then validate the whole.
	merged := monitorInput{
		Name:   firstNonEmpty(in.Name, rec.GetString("name")),
		Type:   firstNonEmpty(in.Type, rec.GetString("type")),
		Target: firstNonEmpty(strings.TrimSpace(in.Target), rec.GetString("target")),
	}
	iv := int(rec.GetFloat("interval"))
	tv := int(rec.GetFloat("timeout"))
	mr := int(rec.GetFloat("max_retries"))
	ra := int(rec.GetFloat("resend_after"))
	nv := rec.GetBool("notify")
	ud := rec.GetBool("upside_down")
	merged.Interval, merged.Timeout, merged.MaxRetries = &iv, &tv, &mr
	merged.ResendAfter, merged.Notify, merged.UpsideDown = &ra, &nv, &ud
	if in.Interval != nil {
		merged.Interval = in.Interval
	}
	if in.Timeout != nil {
		merged.Timeout = in.Timeout
	}
	if in.MaxRetries != nil {
		merged.MaxRetries = in.MaxRetries
	}
	if in.Notify != nil {
		merged.Notify = in.Notify
	}
	if in.ResendAfter != nil {
		merged.ResendAfter = in.ResendAfter
	}
	if in.UpsideDown != nil {
		merged.UpsideDown = in.UpsideDown
	}
	if err := validateInput(&merged); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	prevTarget, prevType := rec.GetString("target"), rec.GetString("type")
	rec.Set("name", merged.Name)
	rec.Set("type", merged.Type)
	rec.Set("target", merged.Target)
	rec.Set("interval", *merged.Interval)
	rec.Set("timeout", *merged.Timeout)
	rec.Set("max_retries", *merged.MaxRetries)
	rec.Set("notify", *merged.Notify)
	rec.Set("resend_after", *merged.ResendAfter)
	rec.Set("upside_down", *merged.UpsideDown)
	if in.Paused != nil && *in.Paused != rec.GetBool("paused") {
		rec.Set("paused", *in.Paused)
		if *in.Paused {
			rec.Set("status", "paused")
		} else {
			rec.Set("status", "pending")
		}
		rec.Set("consecutive_failures", 0)
	}
	if in.Users != nil {
		rec.Set("users", in.Users)
	}
	if in.Config != nil {
		// Shallow top-level merge: nested objects (e.g. headers) are
		// replaced wholesale and keys cannot be deleted individually —
		// send the full config to rewrite it. Secrets sent back redacted
		// keep their stored values.
		existing := map[string]any{}
		_ = rec.UnmarshalJSONField("config", &existing)
		configChanged := false
		for k, v := range in.Config {
			if s, ok := v.(string); ok && s == redactedSecret && secretConfigKeys[strings.ToLower(k)] {
				continue
			}
			existing[k] = v
			configChanged = true
		}
		rec.Set("config", existing)
		if configChanged || merged.Target != prevTarget || merged.Type != prevType {
			rec.Set("consecutive_failures", 0)
		}
	} else if merged.Target != prevTarget || merged.Type != prevType {
		rec.Set("consecutive_failures", 0)
	}
	if err := a.app.Save(rec); err != nil {
		return e.BadRequestError("failed to save monitor", err)
	}
	return e.JSON(http.StatusOK, monitorToResponse(rec))
}

func firstNonEmpty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func (a *MonitorAPI) delete(e *core.RequestEvent) error {
	if e.Auth.GetString("role") == "readonly" {
		return e.ForbiddenError("readonly users cannot delete monitors", nil)
	}
	rec, err := a.findMonitor(e, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	if err := a.app.Delete(rec); err != nil {
		return e.InternalServerError("", err)
	}
	return e.NoContent(http.StatusNoContent)
}

func (a *MonitorAPI) checks(e *core.RequestEvent) error {
	rec, err := a.findMonitor(e, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	limit := 200
	if v := e.Request.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = min(n, 1000)
		}
	}
	// Order + limit are pushed to SQL (newest first); the range cutoff is
	// applied in Go below.
	rows, err := a.app.FindRecordsByFilter("monitor_checks", "monitor = {:mon}", "-created", limit, 0, dbx.Params{"mon": rec.Id})
	if err != nil {
		return e.InternalServerError("failed to load check history", err)
	}
	if rng := e.Request.URL.Query().Get("range"); rng == "24h" || rng == "30d" {
		hours := 24
		if rng == "30d" {
			hours = 30 * 24
		}
		cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
		kept := rows[:0]
		for _, r := range rows {
			if !r.GetDateTime("created").Time().Before(cutoff) {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		details := map[string]any{}
		_ = r.UnmarshalJSONField("details", &details)
		out = append(out, map[string]any{
			"id": r.Id, "status": r.GetString("status"), "latency_ms": r.GetFloat("latency_ms"),
			"code": r.GetRaw("code"), "message": r.GetString("message"), "details": details,
			"cert_days": r.GetRaw("cert_days"), "created": r.GetDateTime("created"),
		})
	}
	return e.JSON(http.StatusOK, out)
}

// allowTest enforces 1 run per 10s per monitor and 10/min per user.
func (a *MonitorAPI) allowTest(userID, monitorID string) bool {
	l := limiterFor(a.app)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	keep := func(ts []time.Time, window time.Duration) []time.Time {
		out := ts[:0]
		for _, t := range ts {
			if now.Sub(t) < window {
				out = append(out, t)
			}
		}
		return out
	}
	user := l.hits["u:"+userID]
	mon := l.hits["m:"+monitorID]
	user = keep(user, time.Minute)
	mon = keep(mon, 10*time.Second)
	if len(user) >= 10 || len(mon) >= 1 {
		l.hits["u:"+userID] = user
		l.hits["m:"+monitorID] = mon
		return false
	}
	l.hits["u:"+userID] = append(user, now)
	l.hits["m:"+monitorID] = append(mon, now)
	return true
}

func (a *MonitorAPI) test(e *core.RequestEvent) error {
	if e.Auth.GetString("role") == "readonly" {
		return e.ForbiddenError("readonly users cannot test monitors", nil)
	}
	rec, err := a.findMonitor(e, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	if !a.allowTest(e.Auth.Id, rec.Id) {
		return e.JSON(429, map[string]any{"error": "rate limited, try again later"})
	}
	mr := recordToMonitor(rec)
	timeout := time.Duration(mr.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(e.Request.Context(), timeout)
	defer cancel()
	res := RunCheck(ctx, mr.ToMonitor())
	out := map[string]any{
		"status": res.Status, "latency_ms": res.LatencyMs, "message": res.Message,
		"details": res.Details,
	}
	if res.Code != nil {
		out["code"] = *res.Code
	}
	if res.CertDays != nil {
		out["cert_days"] = *res.CertDays
	}
	return e.JSON(http.StatusOK, out)
}

func (a *MonitorAPI) summary(e *core.RequestEvent) error {
	recs, err := a.app.FindAllRecords("monitors")
	if err != nil {
		return e.InternalServerError("failed to load monitors", err)
	}
	recs = filterMonitorRecords(recs, e.Auth.Id, "")
	counts := map[string]int{"up": 0, "down": 0, "warn": 0, "paused": 0, "pending": 0}
	var down []map[string]any
	for _, rec := range recs {
		st := rec.GetString("status")
		if rec.GetBool("paused") {
			st = "paused"
		}
		counts[st]++
		if st == "down" {
			down = append(down, map[string]any{"id": rec.Id, "name": rec.GetString("name")})
		}
	}
	if down == nil {
		down = []map[string]any{}
	}
	return e.JSON(http.StatusOK, map[string]any{"counts": counts, "down": down})
}
