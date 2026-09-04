package monitors

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testMonitor() Monitor {
	return Monitor{Name: "t", Type: TypeHTTP, Target: "https://example.com", IntervalSeconds: 60, TimeoutSeconds: 10, MaxRetries: 2}
}

func upResult() CheckResult   { return CheckResult{Status: StatusUp, LatencyMs: 12} }
func downResult() CheckResult { return CheckResult{Status: StatusDown, Message: "boom"} }

func TestStateMachine_DownAfterMaxRetriesPlusOne(t *testing.T) {
	st := newMonitorState(testMonitor())
	for i := 0; i < 2; i++ {
		_, _ = st.applyResult(downResult())
		if st.status() != StatusUp {
			t.Fatalf("run %d: expected still up, got %q", i+1, st.status())
		}
	}
	_, _ = st.applyResult(downResult())
	if st.status() != StatusDown {
		t.Fatalf("expected down after 3rd failure, got %q", st.status())
	}
}

func TestStateMachine_SuccessResetsFailures(t *testing.T) {
	st := newMonitorState(testMonitor())
	_, _ = st.applyResult(downResult())
	_, _ = st.applyResult(upResult())
	if st.failures() != 0 {
		t.Fatalf("expected failures reset, got %d", st.failures())
	}
	if st.status() != StatusUp {
		t.Fatalf("expected up, got %q", st.status())
	}
}

func TestStateMachine_ZeroRetriesDownImmediately(t *testing.T) {
	m := testMonitor()
	m.MaxRetries = 0
	st := newMonitorState(m)
	_, _ = st.applyResult(downResult())
	if st.status() != StatusDown {
		t.Fatalf("expected immediate down, got %q", st.status())
	}
}

func TestStateMachine_UpsideDownInverts(t *testing.T) {
	m := testMonitor()
	m.UpsideDown = true
	st := newMonitorState(m)
	// Raw successes invert to failures: debounced status stays up for the
	// first max_retries inverted failures, then flips to down.
	for i := 0; i < 2; i++ {
		res, _ := st.applyResult(upResult())
		if res.Status != StatusUp {
			t.Fatalf("run %d: expected still up, got %q", i+1, res.Status)
		}
	}
	if got, _ := st.applyResult(upResult()); got.Status != StatusDown {
		t.Fatalf("expected inverted down, got %q", got.Status)
	}
}

func TestStateMachine_WarnDoesNotFlipUpDown(t *testing.T) {
	st := newMonitorState(testMonitor())
	_, _ = st.applyResult(CheckResult{Status: StatusWarn, Message: "expiring"})
	if st.status() != StatusWarn {
		t.Fatalf("expected warn, got %q", st.status())
	}
	_, _ = st.applyResult(upResult())
	if st.status() != StatusUp {
		t.Fatalf("expected up after warn, got %q", st.status())
	}
}

func TestStateMachine_DeliversDebouncedStatus(t *testing.T) {
	st := newMonitorState(testMonitor())
	res, changed := st.applyResult(downResult())
	if res.Status != StatusUp {
		t.Fatalf("single failure must still deliver up, got %q", res.Status)
	}
	if changed {
		t.Fatal("no transition expected on first failure")
	}
	_, _ = st.applyResult(downResult())
	res, changed = st.applyResult(downResult())
	if res.Status != StatusDown {
		t.Fatalf("expected debounced down, got %q", res.Status)
	}
	if !changed {
		t.Fatal("transition expected on flip to down")
	}
}

func TestStateMachine_RunCheckDispatches(t *testing.T) {
	m := testMonitor()
	m.Type = "nope"
	res := RunCheck(context.Background(), m)
	if res.Status != StatusDown {
		t.Fatalf("expected down for unknown type, got %q", res.Status)
	}
}

func TestManager_RunOnceDeliversDown(t *testing.T) {
	m := testMonitor()
	m.MaxRetries = 0
	m.IntervalSeconds = 3600
	gotCh := make(chan CheckResult, 1)
	mgr := NewManager(2, func(ctx context.Context, mon Monitor) CheckResult { return downResult() }, func(mon Monitor, res CheckResult, _ bool) { gotCh <- res })
	mgr.jitterMax = 0
	defer mgr.Stop()
	mgr.Add(m)
	mgr.mu.Lock()
	mm := mgr.monitors[m.Name]
	mgr.mu.Unlock()
	mgr.runOnce(mm)
	select {
	case got := <-gotCh:
		if got.Status != StatusDown {
			t.Fatalf("expected down via onResult, got %q", got.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected down result via onResult")
	}
}

func TestManager_SkipsOverlap(t *testing.T) {
	m := testMonitor()
	// Long interval + zero jitter: the background loop never fires during
	// the test, so only the manually driven runs below exist.
	m.IntervalSeconds = 3600
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	mgr := NewManager(2, func(ctx context.Context, mon Monitor) CheckResult {
		started <- struct{}{}
		<-release
		return upResult()
	}, nil)
	mgr.jitterMax = 0
	defer mgr.Stop()
	mgr.Add(m)
	mgr.mu.Lock()
	mm := mgr.monitors[m.Name]
	mgr.mu.Unlock()
	// First run blocks inside check; second must skip on the busy flag.
	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.runOnce(mm)
	}()
	<-started
	mgr.runOnce(mm)
	close(release)
	<-done
	if n := mgr.Skipped(m.Name); n == 0 {
		t.Fatal("expected skipped runs from overlap")
	}
}

func TestManager_RecoversCheckerPanic(t *testing.T) {
	m := testMonitor()
	m.IntervalSeconds = 3600
	mgr := NewManager(2, func(ctx context.Context, mon Monitor) CheckResult { panic("boom") }, nil)
	mgr.jitterMax = 0
	defer mgr.Stop()
	mgr.Add(m)
	time.Sleep(100 * time.Millisecond)
	mgr.mu.Lock()
	mm := mgr.monitors[m.Name]
	mgr.mu.Unlock()
	mgr.runOnce(mm)
	if got := mm.state.status(); got != StatusUp {
		t.Fatalf("panic should count as failure without flipping yet, got %q", got)
	}
	// The background loop may also have run once (immediate first run), so
	// assert at least one recorded failure rather than an exact count.
	if n := mm.state.failures(); n < 1 {
		t.Fatalf("expected >= 1 failure after panic, got %d", n)
	}
}

func TestManager_SemaphoreBoundsConcurrency(t *testing.T) {
	var current, peak atomic.Int32
	mgr := NewManager(2, func(ctx context.Context, mon Monitor) CheckResult {
		n := current.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		current.Add(-1)
		return upResult()
	}, nil)
	defer mgr.Stop()
	names := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		m := testMonitor()
		m.Name = "c" + string(rune('a'+i))
		m.IntervalSeconds = 3600
		names = append(names, m.Name)
		mgr.Add(m)
	}
	// Drive all five runs concurrently by hand; the semaphore must cap
	// simultaneous check executions at 2.
	var wg sync.WaitGroup
	for _, name := range names {
		mgr.mu.Lock()
		mm := mgr.monitors[name]
		mgr.mu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.runOnce(mm)
		}()
	}
	wg.Wait()
	if p := peak.Load(); p > 2 {
		t.Fatalf("expected peak concurrency <= 2, got %d", p)
	}
}

func TestManager_SurvivesOnResultPanic(t *testing.T) {
	m := testMonitor()
	m.MaxRetries = 0
	m.IntervalSeconds = 3600
	calls := make(chan struct{}, 4)
	mgr := NewManager(2, func(ctx context.Context, mon Monitor) CheckResult { return upResult() }, func(mon Monitor, res CheckResult, _ bool) {
		calls <- struct{}{}
		panic("db write failed")
	})
	mgr.jitterMax = 0
	defer mgr.Stop()
	mgr.Add(m)
	mgr.mu.Lock()
	mm := mgr.monitors[m.Name]
	mgr.mu.Unlock()
	// A panicking onResult must degrade to a failed attempt, and the monitor
	// must still accept subsequent runs (loop alive).
	mgr.runOnce(mm)
	select {
	case <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("expected first onResult call")
	}
	mgr.runOnce(mm)
	select {
	case <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("loop died after onResult panic: second run never delivered")
	}
}
