package agent

import (
	"sync"
	"time"
)

const (
	monitorResumeHeartbeat = 10 * time.Second
	// Allow scheduling jitter without mistaking an ordinary tick for resume.
	monitorResumeGap   = 2 * monitorResumeHeartbeat
	monitorResumePause = 10 * time.Second
)

// monitorResumeGuard detects likely suspend/resume using wall time. A long
// process stall or forward clock adjustment can also trigger the bounded pause.
// One heartbeat is shared by all configured monitors.
type monitorResumeGuard struct {
	mu         sync.Mutex
	stop       chan struct{}
	lastTick   time.Time
	pauseUntil time.Time
	generation uint32
}

func (g *monitorResumeGuard) start() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stop != nil {
		return
	}
	stop := make(chan struct{})
	g.stop = stop
	g.lastTick = time.Now().Round(0)
	g.pauseUntil = time.Time{}
	go func() {
		ticker := time.NewTicker(monitorResumeHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				g.mu.Lock()
				if g.stop == stop {
					g.observe(time.Now())
				}
				g.mu.Unlock()
			}
		}
	}()
}

func (g *monitorResumeGuard) shutdown() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stop != nil {
		close(g.stop)
		g.stop = nil
		g.generation++
	}
}

// observe requires mu. Strip the monotonic component because it can stop during
// suspend. Read the current time rather than the ticker's queued timestamp.
func (g *monitorResumeGuard) observe(now time.Time) {
	now = now.Round(0)
	if now.Sub(g.lastTick) > monitorResumeGap {
		g.pauseUntil = now.Add(monitorResumePause)
		g.generation++
	}
	g.lastTick = now
}

// snapshot also observes time so a probe waking before the heartbeat detects
// resume itself. A changed generation invalidates probes spanning suspend.
func (g *monitorResumeGuard) snapshot() (generation uint32, allowed bool) {
	if g == nil {
		return 0, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stop == nil {
		return g.generation, true
	}
	g.observe(time.Now())
	return g.generation, !g.lastTick.Before(g.pauseUntil)
}
