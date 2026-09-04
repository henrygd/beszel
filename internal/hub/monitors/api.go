package monitors

import (
	"context"
	"fmt"
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
	app      core.App
	mu       sync.Mutex
	testHits map[string][]time.Time
}

// RegisterRoutes mounts the monitors API under /api/beszel/monitors via an
// OnServe hook, so it works both in production and in ApiScenario tests.
func RegisterRoutes(app core.App) {
	api := &MonitorAPI{app: app, testHits: make(map[string][]time.Time)}
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/beszel/monitors")
		g.Bind(apis.RequireAuth())
		g.GET("", api.list)
		g.POST("", api.create)
		g.GET("/summary", api.summary)
		g.GET("/{id}", api.get)
		g.PATCH("/{id}", api.update)
		g.DELETE("/{id}", api.delete)
		g.GET("/{id}/checks", api.checks)
		g.POST("/{id}/test", api.test)
		return e.Next()
	})
}

type monitorInput struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Target      string         `json:"target"`
	Interval    *int           `json:"interval"`
	Timeout     *int           `json:"timeout"`
	MaxRetries  *int           `json:"max_retries"`
	UpsideDown  bool           `json:"upside_down"`
	Paused      bool           `json:"paused"`
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

// validateInput applies defaults and returns field-typed errors.
func validateInput(in *monitorInput) error {
	m := Monitor{
		Name: in.Name, Type: MonitorType(in.Type), Target: strings.TrimSpace(in.Target),
		IntervalSeconds: intOr(in.Interval, 60), TimeoutSeconds: intOr(in.Timeout, 10),
		MaxRetries: intOr(in.MaxRetries, 2), UpsideDown: in.UpsideDown,
	}
	if in.Name == "" {
		return fmt.Errorf("name is required")
	}
	return m.Validate()
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
	out := recs[:0]
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
		return e.BadRequestError("", err)
	}
	return e.JSON(http.StatusOK, monitorToResponse(rec))
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
	merged.Interval, merged.Timeout, merged.MaxRetries = &iv, &tv, &mr
	merged.ResendAfter, merged.Notify = &ra, &nv
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
	merged.UpsideDown = rec.GetBool("upside_down")
	if in.UpsideDown {
		merged.UpsideDown = true
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
	if in.Paused != rec.GetBool("paused") {
		rec.Set("paused", in.Paused)
		rec.Set("status", "paused")
		rec.Set("consecutive_failures", 0)
	}
	if in.Users != nil {
		rec.Set("users", in.Users)
	}
	if in.Config != nil {
		// PATCH without secrets keeps existing secrets.
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
		return e.BadRequestError("", err)
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
	filter := "monitor = {:mon}"
	params := dbx.Params{"mon": rec.Id}
	if rng := e.Request.URL.Query().Get("range"); rng == "24h" || rng == "30d" {
		hours := 24
		if rng == "30d" {
			hours = 30 * 24
		}
		filter += " && created >= {:cutoff}"
		params["cutoff"] = time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	}
	rows, err := a.app.FindAllRecords("monitor_checks", dbx.NewExp(filter, params))
	if err != nil {
		return e.InternalServerError("", err)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].GetDateTime("created").After(rows[j].GetDateTime("created"))
	})
	if len(rows) > limit {
		rows = rows[:limit]
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
	a.mu.Lock()
	defer a.mu.Unlock()
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
	user := a.testHits["u:"+userID]
	mon := a.testHits["m:"+monitorID]
	user = keep(user, time.Minute)
	mon = keep(mon, 10*time.Second)
	if len(user) >= 10 || len(mon) >= 1 {
		a.testHits["u:"+userID] = user
		a.testHits["m:"+monitorID] = mon
		return false
	}
	a.testHits["u:"+userID] = append(user, now)
	a.testHits["m:"+monitorID] = append(mon, now)
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
		return e.InternalServerError("", err)
	}
	recs = filterMonitorRecords(recs, e.Auth.Id, "")
	if err != nil {
		return e.InternalServerError("", err)
	}
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
