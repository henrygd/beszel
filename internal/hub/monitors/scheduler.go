package monitors

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMaxConcurrent caps simultaneous check runs across all monitors.
const DefaultMaxConcurrent = 10

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
// effective result after upside_down inversion and retry counting.
func (s *monitorState) applyResult(res CheckResult) CheckResult {
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
	return res
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
	onResult  func(m Monitor, res CheckResult)
	jitterMax time.Duration
}

type managedMonitor struct {
	monitor Monitor
	state   *monitorState
	ctx     context.Context
	cancel  context.CancelFunc
	running atomic.Bool
	skipped uint64
}

// NewManager creates a Manager. maxConcurrent <= 0 selects DefaultMaxConcurrent.
func NewManager(maxConcurrent int, check CheckFunc, onResult func(m Monitor, res CheckResult)) *Manager {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrent
	}
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

// Add starts scheduling checks for m (keyed by name+target). Re-adding
// replaces the previous entry.
func (mgr *Manager) Add(m Monitor) {
	mgr.Remove(m.Name)
	ctx, cancel := context.WithCancel(context.Background())
	mm := &managedMonitor{monitor: m, state: newMonitorState(m), ctx: ctx, cancel: cancel}
	mgr.mu.Lock()
	mgr.monitors[m.Name] = mm
	mgr.mu.Unlock()
	go mgr.loop(mm)
}

// Remove stops scheduling for the named monitor.
func (mgr *Manager) Remove(name string) {
	mgr.mu.Lock()
	mm, ok := mgr.monitors[name]
	if ok {
		delete(mgr.monitors, name)
	}
	mgr.mu.Unlock()
	if ok {
		mm.cancel()
	}
}

// Stop cancels all scheduled monitors.
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
}

// Skipped returns the number of ticks skipped due to overlap for a monitor.
func (mgr *Manager) Skipped(name string) uint64 {
	mgr.mu.Lock()
	mm, ok := mgr.monitors[name]
	mgr.mu.Unlock()
	if !ok {
		return 0
	}
	return atomic.LoadUint64(&mm.skipped)
}

func (mgr *Manager) loop(mm *managedMonitor) {
	interval := time.Duration(mm.monitor.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	// Initial jitter spreads load when many monitors start together.
	if mgr.jitterMax > 0 {
		select {
		case <-mm.ctx.Done():
			return
		case <-time.After(time.Duration(rand.Int64N(int64(mgr.jitterMax)))):
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
		atomic.AddUint64(&mm.skipped, 1)
		return
	}
	defer mm.running.Store(false)
	select {
	case mgr.sem <- struct{}{}:
		defer func() { <-mgr.sem }()
	default:
		atomic.AddUint64(&mm.skipped, 1)
		return
	}
	timeout := time.Duration(mm.monitor.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultCheckTimeout
	}
	ctx, cancel := context.WithTimeout(mm.ctx, timeout)
	defer cancel()
	res := func() (res CheckResult) {
		defer func() {
			if r := recover(); r != nil {
				res = CheckResult{Status: StatusDown, Message: fmt.Sprintf("checker panic: %v", r)}
			}
		}()
		return mgr.check(ctx, mm.monitor)
	}()
	res = mm.state.applyResult(res)
	if mgr.onResult != nil {
		mgr.onResult(mm.monitor, res)
	}
}
