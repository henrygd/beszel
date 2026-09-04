package monitors

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// AlertSender delivers transition notifications. It mirrors
// alerts.AlertManager.SendAlert over (userID, title, message, link) so the
// engine stays decoupled from the alerts package (wired in hub.go).
type AlertSender func(userID, title, message, link string)

// Engine connects the scheduler, persistence and notifications for the
// monitors stored in PocketBase.
type Engine struct {
	app    core.App
	mgr    *Manager
	send   AlertSender
	link   func(monitorID string) string
	mu     sync.Mutex
	sentAt map[string]time.Time
	// downSince tracks when the current DOWN stretch started (for recovery
	// duration and resend decisions).
	downSince map[string]time.Time
	started   atomic.Bool
}

// NewEngine creates an Engine. send may be nil (persist only, no notify).
func NewEngine(app core.App, send AlertSender) *Engine {
	e := &Engine{
		app:       app,
		sentAt:    make(map[string]time.Time),
		downSince: make(map[string]time.Time),
		send:      send,
		link:      func(id string) string { return "/monitors/" + id },
	}
	e.mgr = NewManager(0, nil, e.handleResult)
	return e
}

// SetCheck replaces the check function (tests; production uses RunCheck).
func (e *Engine) SetCheck(check CheckFunc) {
	e.mgr.check = check
}

// SetLink replaces the notification link formatter.
func (e *Engine) SetLink(link func(string) string) {
	e.link = link
}

// TestAdd schedules m with check (testing only: bypasses validation and
// stagger for fast loop-driven tests).
func (e *Engine) TestAdd(m Monitor, check CheckFunc) {
	e.mgr.check = check
	e.mgr.jitterMax = 0
	e.mgr.AddID(m.ID, m)
}

// Start loads non-paused monitors with stagger and begins scheduling. It
// also binds record hooks so UI/API changes reschedule without restart.
// Calling Start twice is a no-op for hooks (loops are replace-safe).
func (e *Engine) Start() error {
	if !e.started.CompareAndSwap(false, true) {
		return nil
	}
	recs, err := LoadMonitors(e.app)
	if err != nil {
		return err
	}
	interval := 60 * time.Second
	delay := StaggerDelay(interval, len(recs))
	// Cap total boot stagger: per-monitor loops already jitter 0-5s, so a
	// few seconds here only avoid a thundering herd without delaying serve.
	if d := delay * time.Duration(max(1, len(recs))); d > 5*time.Second && len(recs) > 0 {
		delay = 5 * time.Second / time.Duration(len(recs))
	}
	for i, mr := range recs {
		if i > 0 && delay > 0 {
			time.Sleep(delay)
		}
		if err := mr.ToMonitor().Validate(); err != nil {
			slog.Warn("monitors: skipping invalid monitor", "monitor", mr.Name, "err", err)
			continue
		}
		e.mgr.AddRecord(mr)
	}
	e.app.OnRecordAfterCreateSuccess("monitors").BindFunc(e.onCreate)
	e.app.OnRecordAfterUpdateSuccess("monitors").BindFunc(e.onUpdate)
	e.app.OnRecordAfterDeleteSuccess("monitors").BindFunc(e.onDelete)
	e.app.OnTerminate().BindFunc(func(ev *core.TerminateEvent) error {
		e.Stop()
		return ev.Next()
	})
	return nil
}

func (e *Engine) onCreate(ev *core.RecordEvent) error {
	mr := recordToMonitor(ev.Record)
	if mr.Paused {
		return ev.Next()
	}
	if err := mr.ToMonitor().Validate(); err != nil {
		return ev.Next()
	}
	e.mgr.AddRecord(mr)
	return ev.Next()
}

func (e *Engine) onUpdate(ev *core.RecordEvent) error {
	mr := recordToMonitor(ev.Record)
	// Scheduler writes (status, last_check, latency, uptime, failures) on
	// every cycle via SaveNoValidate: rescheduling on those would churn
	// Remove+Add per cycle AND deadlock (Remove waits for the loop that is
	// currently inside runOnce). Only reschedule when schedule-relevant
	// fields changed.
	if !scheduleRelevantChange(ev.Record) {
		if mr.Paused {
			e.mu.Lock()
			delete(e.sentAt, mr.ID)
			delete(e.downSince, mr.ID)
			e.mu.Unlock()
		}
		return ev.Next()
	}
	e.mgr.Remove(mr.ID)
	if mr.Paused {
		// Pause stops the loop; drop resend/down bookkeeping. The
		// consecutive_failures reset is written by the API PATCH handler,
		// the only writer of the paused flag (writing here would recurse
		// into this hook).
		e.mu.Lock()
		delete(e.sentAt, mr.ID)
		delete(e.downSince, mr.ID)
		e.mu.Unlock()
		return ev.Next()
	}
	if err := mr.ToMonitor().Validate(); err != nil {
		return ev.Next()
	}
	e.mgr.AddRecord(mr)
	return ev.Next()
}

func (e *Engine) onDelete(ev *core.RecordEvent) error {
	e.mgr.Remove(ev.Record.Id)
	e.mu.Lock()
	delete(e.sentAt, ev.Record.Id)
	delete(e.downSince, ev.Record.Id)
	e.mu.Unlock()
	return ev.Next()
}

// scheduleRelevantChange reports whether schedule-driving fields changed.
// Status/latency/uptime/failure writes from the scheduler itself must not
// reschedule (churn + self-join deadlock via Remove's wg.Wait).
func scheduleRelevantChange(rec *core.Record) bool {
	orig := rec.Original()
	if orig == nil {
		return true
	}
	for _, f := range []string{"name", "type", "target", "interval", "timeout", "max_retries", "upside_down", "paused", "config"} {
		if !recordFieldEqual(orig.Get(f), rec.Get(f)) {
			return true
		}
	}
	return false
}

func recordFieldEqual(a, b any) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// SyncOne runs one persisted check cycle for a monitor id. It loads the
// record fresh (paused monitors are skipped), runs the check with timeout,
// folds it through a state seeded from consecutive_failures, persists the
// cycle in one transaction, and notifies on transitions.
func (e *Engine) SyncOne(id string) error {
	rec, err := e.app.FindRecordById("monitors", id)
	if err != nil {
		return err
	}
	mr := recordToMonitor(rec)
	if mr.Paused {
		return nil
	}
	m := mr.ToMonitor()
	timeout := time.Duration(m.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	check := RunCheck
	if e.mgr.check != nil {
		check = e.mgr.check
	}
	res := check(ctx, m)
	st := newMonitorState(m)
	st.consec = mr.ConsecutiveFailure
	// Seed from the stored status so a fresh state still sees transitions
	// (e.g. stored down followed by up must notify recovery).
	switch mr.Status {
	case StatusUp, StatusDown, StatusWarn:
		st.current = mr.Status
	}
	res, transition := st.applyResult(res)
	e.persistAndNotify(mr, res, st.failures(), transition)
	return nil
}

// handleResult is the scheduler callback. Monitors scheduled via AddID
// carry their record id, so the row is resolved without name lookups.
func (e *Engine) handleResult(m Monitor, res CheckResult, transition bool, failures int) {
	if m.ID == "" {
		slog.Debug("monitors: dropping result without record id", "monitor", m.Name)
		return
	}
	rec, err := e.app.FindRecordById("monitors", m.ID)
	if err != nil {
		slog.Debug("monitors: dropping result for deleted monitor", "id", m.ID, "err", err)
		return
	}
	mr := recordToMonitor(rec)
	e.persistAndNotify(mr, res, failures, transition)
}

// persistAndNotify persists one cycle then notifies.
// mr identifies the monitor row; res is debounced; failures is exact.
// Notification rules: transitions (up→down, down→up, warn in/out) always
// notify when mr.Notify; a steady DOWN renotifies only after ResendAfter
// minutes since the last DOWN notice (0 = never).
func (e *Engine) persistAndNotify(mr MonitorRecord, res CheckResult, failures int, transition bool) {
	if err := SaveCheckResult(e.app, mr, res, failures, transition); err != nil {
		slog.Error("monitors: failed to save check result", "monitor", mr.Name, "err", err)
		return
	}
	if !mr.Notify || e.send == nil {
		return
	}
	now := time.Now()
	e.mu.Lock()
	if _, ok := e.downSince[mr.ID]; !ok && res.Status == StatusDown {
		e.downSince[mr.ID] = now
	}
	notify := transition
	if res.Status == StatusDown && mr.ResendAfter > 0 {
		if last, ok := e.sentAt[mr.ID]; !ok || now.Sub(last) >= time.Duration(mr.ResendAfter)*time.Minute {
			notify = true
		}
	}
	if notify {
		e.sentAt[mr.ID] = now
	}
	downStart, wasDown := e.downSince[mr.ID]
	if res.Status != StatusDown {
		delete(e.downSince, mr.ID)
		delete(e.sentAt, mr.ID)
	}
	e.mu.Unlock()
	if !notify {
		return
	}

	var title, message string
	switch res.Status {
	case StatusDown:
		title = fmt.Sprintf("Monitor down: %s", mr.Name)
		message = fmt.Sprintf("%s is down: %s (latency %.1f ms)", mr.Target, res.Message, res.LatencyMs)
	case StatusWarn:
		title = fmt.Sprintf("Monitor warning: %s", mr.Name)
		message = fmt.Sprintf("%s: %s", mr.Target, res.Message)
		if res.CertDays != nil {
			message += fmt.Sprintf(" (%.0f days left)", *res.CertDays)
		}
	default:
		title = fmt.Sprintf("Monitor recovered: %s", mr.Name)
		message = fmt.Sprintf("%s is back up", mr.Target)
		if wasDown {
			message += fmt.Sprintf(" after %s", now.Sub(downStart).Round(time.Second))
		}
		if uptime, err := Uptime24h(e.app, mr.ID); err == nil && uptime > 0 {
			message += fmt.Sprintf(" (24h uptime %.1f%%)", uptime)
		}
	}
	for _, userID := range mr.UserIDs {
		e.send(userID, title, message, e.link(mr.ID))
	}
}

// Stop halts all scheduled monitors.
func (e *Engine) Stop() {
	e.mgr.Stop()
}
