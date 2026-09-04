package monitors

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMaxConcurrent caps simultaneous check runs across all monitors.
const (
	DefaultMaxConcurrent = 10
	MaxConcurrentCap     = 50
	// maxConcurrentEnv overrides the semaphore size (admin-only, lab use).
	maxConcurrentEnv = "MONITORS_MAX_CONCURRENT"
)

// monitorState tracks consecutive failures and the current status of one
// monitor. It is the single place where raw check results become a status,
// including upside_down inversion.
type monitorState struct {
	mu      sync.Mutex
	monitor Monitor
	consec  int
	current string
	warn    bool
}

// newMonitorState creates state starting from up (fresh monitor).
func newMonitorState(m Monitor) *monitorState {
	return &monitorState{monitor: m, current: StatusUp}
}

// applyResult folds a raw check result into the state and returns the
// effective result after upside_down inversion and retry debouncing: the
// returned Status is the debounced state, so transient single failures do
// not surface as down while failures <= max_retries. It also reports
// whether the debounced status changed on this attempt (for transitions).
func (s *monitorState) applyResult(res CheckResult) (CheckResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.monitor.UpsideDown {
		switch res.Status {
		case StatusUp:
			res.Status = StatusDown
		case StatusDown:
			res.Status = StatusUp
		}
	}
	prev := s.current
	switch res.Status {
	case StatusUp:
		s.consec = 0
		s.current = StatusUp
		s.warn = false
	case StatusWarn:
		s.consec = 0
		s.current = StatusWarn
		s.warn = true
	default:
		s.consec++
		if s.consec > s.monitor.MaxRetries {
			s.current = StatusDown
		}
	}
	res.Status = s.current
	return res, s.current != prev
}

func (s *monitorState) status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *monitorState) failures() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.consec
}

// RunCheck dispatches to the checker matching the monitor type.
func RunCheck(ctx context.Context, m Monitor) CheckResult {
	switch m.Type {
	case TypeHTTP, TypeKeyword:
		return CheckHTTP(ctx, m)
	case TypeTLS:
		return CheckTLS(ctx, m)
	case TypeDNS:
		return CheckDNS(ctx, m)
	case TypePing:
		return CheckPing(ctx, m)
	default:
		return CheckResult{Status: StatusDown, Message: fmt.Sprintf("unknown monitor type %q", m.Type)}
	}
}

// CheckFunc performs one check attempt. It defaults to RunCheck and is
// replaceable in tests.
type CheckFunc func(ctx context.Context, m Monitor) CheckResult

// Manager schedules periodic checks for a set of monitors.
type Manager struct {
	mu        sync.Mutex
	monitors  map[string]*managedMonitor
	sem       chan struct{}
	maxConc   int
	check     CheckFunc
	onResult  func(m Monitor, res CheckResult, transition bool)
	jitterMax time.Duration
}

// resolveMaxConcurrent applies the admin env override, default and cap.
func resolveMaxConcurrent(n int) int {
	if env, ok := os.LookupEnv(maxConcurrentEnv); ok {
		if parsed, err := strconv.Atoi(env); err == nil {
			n = parsed
		}
	}
	if n <= 0 {
		n = DefaultMaxConcurrent
	}
	if n > MaxConcurrentCap {
		n = MaxConcurrentCap
	}
	return n
}

type managedMonitor struct {
	monitor Monitor
	state   *monitorState
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running atomic.Bool
	skipped atomic.Uint64
	// saturated counts ticks dropped because the global semaphore was full
	// (kept separate from overlap skips).
	saturated atomic.Uint64
}

// NewManager creates a Manager. maxConcurrent <= 0 selects
// DefaultMaxConcurrent; the MONITORS_MAX_CONCURRENT env var overrides,
// clamped to [1, MaxConcurrentCap].
func NewManager(maxConcurrent int, check CheckFunc, onResult func(m Monitor, res CheckResult, transition bool)) *Manager {
	maxConcurrent = resolveMaxConcurrent(maxConcurrent)
	if check == nil {
		check = RunCheck
	}
	return &Manager{
		monitors:  make(map[string]*managedMonitor),
		sem:       make(chan struct{}, maxConcurrent),
		maxConc:   maxConcurrent,
		check:     check,
		onResult:  onResult,
		jitterMax: 5 * time.Second,
	}
}

// Add starts scheduling checks for m, keyed by name. Re-adding cancels the
// previous entry and waits for its loop before replacing it, so no orphaned
// goroutine survives. Callers loading many monitors at boot should stagger
// Add calls (see StaggerDelay); Task 8 keys by monitor ID instead to avoid
// same-name collisions across users.
func (mgr *Manager) Add(m Monitor) {
	mm := &managedMonitor{monitor: m, state: newMonitorState(m)}
	mm.ctx, mm.cancel = context.WithCancel(context.Background())
	mm.wg.Add(1)
	mgr.mu.Lock()
	old, dup := mgr.monitors[m.Name]
	mgr.monitors[m.Name] = mm
	mgr.mu.Unlock()
	if dup {
		old.cancel()
		old.wg.Wait()
	}
	go mgr.loop(mm)
}

// StaggerDelay returns the boot stagger between Add calls for n monitors,
// mirroring SystemManager: interval/n capped at 2 seconds.
func StaggerDelay(interval time.Duration, n int) time.Duration {
	if n <= 0 {
		return 0
	}
	d := interval / time.Duration(n)
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	return d
}

// Remove stops scheduling for the named monitor and waits for its loop,
// so no stale onResult delivery happens after return.
func (mgr *Manager) Remove(name string) {
	mgr.mu.Lock()
	mm, ok := mgr.monitors[name]
	if ok {
		delete(mgr.monitors, name)
	}
	mgr.mu.Unlock()
	if ok {
		mm.cancel()
		mm.wg.Wait()
	}
}

// Stop cancels all scheduled monitors and waits for every loop.
func (mgr *Manager) Stop() {
	mgr.mu.Lock()
	mms := make([]*managedMonitor, 0, len(mgr.monitors))
	for _, mm := range mgr.monitors {
		mms = append(mms, mm)
	}
	mgr.monitors = make(map[string]*managedMonitor)
	mgr.mu.Unlock()
	for _, mm := range mms {
		mm.cancel()
	}
	for _, mm := range mms {
		mm.wg.Wait()
	}
}

// Skipped returns overlap skips for a monitor.
func (mgr *Manager) Skipped(name string) uint64 {
	return mgr.skipped(name, false)
}

// Saturated returns semaphore-saturation drops for a monitor.
func (mgr *Manager) Saturated(name string) uint64 {
	return mgr.skipped(name, true)
}

func (mgr *Manager) skipped(name string, saturated bool) uint64 {
	mgr.mu.Lock()
	mm, ok := mgr.monitors[name]
	mgr.mu.Unlock()
	if !ok {
		return 0
	}
	if saturated {
		return mm.saturated.Load()
	}
	return mm.skipped.Load()
}

func (mgr *Manager) loop(mm *managedMonitor) {
	defer mm.wg.Done()
	// Recover so one bad callback never kills a monitor silently.
	defer func() {
		if r := recover(); r != nil {
			slog.Error("monitors: loop panic recovered", "monitor", mm.monitor.Name, "panic", r)
		}
	}()
	interval := time.Duration(mm.monitor.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	// Initial jitter spreads load when many monitors start together.
	if mgr.jitterMax > 0 {
		timer := time.NewTimer(time.Duration(rand.Int64N(int64(mgr.jitterMax))))
		select {
		case <-mm.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	mgr.runOnce(mm)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-mm.ctx.Done():
			return
		case <-ticker.C:
			mgr.runOnce(mm)
		}
	}
}

func (mgr *Manager) runOnce(mm *managedMonitor) {
	if !mm.running.CompareAndSwap(false, true) {
		mm.skipped.Add(1)
		return
	}
	defer mm.running.Store(false)
	select {
	case mgr.sem <- struct{}{}:
		defer func() { <-mgr.sem }()
	default:
		mm.saturated.Add(1)
		return
	}
	timeout := time.Duration(mm.monitor.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	ctx, cancel := context.WithTimeout(mm.ctx, timeout)
	defer cancel()
	// The recover covers check, state fold and delivery so a panicking
	// onResult (e.g. a failing DB write in Task 8) degrades to a failed
	// attempt instead of killing the monitor's loop silently.
	res, transition := func() (res CheckResult, transition bool) {
		defer func() {
			if r := recover(); r != nil {
				res = CheckResult{Status: StatusDown, Message: fmt.Sprintf("checker panic: %v", r)}
				res, transition = mm.state.applyResult(res)
			}
		}()
		res = mgr.check(ctx, mm.monitor)
		return mm.state.applyResult(res)
	}()
	// Skip delivery if the monitor was removed mid-run: the loop may still
	// be inside runOnce when Remove/Stop cancels.
	mgr.mu.Lock()
	current, ok := mgr.monitors[mm.monitor.Name]
	stale := !ok || current != mm
	mgr.mu.Unlock()
	if stale {
		return
	}
	if mgr.onResult != nil {
		mgr.onResult(mm.monitor, res, transition)
	}
}
