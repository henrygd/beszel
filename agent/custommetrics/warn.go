package custommetrics

import (
	"log/slog"
	"time"
)

// warnInterval is how long a warning stays quiet before it is logged again.
const warnInterval = time.Hour

// logWarn and logInfo log a warning and a notice. Tests replace them to capture output.
var (
	logWarn = slog.Warn
	logInfo = slog.Info
)

// warnLimiter logs each problem at most once per warnInterval, so a broken
// producer cannot flood the log on every collection. Keys name the subject,
// e.g. parse:<file>, collision:<file> or missing:<path>.
type warnLimiter struct {
	last map[string]time.Time
}

func newWarnLimiter() *warnLimiter {
	return &warnLimiter{last: make(map[string]time.Time)}
}

// warn logs msg unless the same key was logged less than warnInterval ago.
func (w *warnLimiter) warn(now time.Time, key, msg string, args ...any) {
	if last, ok := w.last[key]; ok && now.Sub(last) < warnInterval {
		return
	}
	if len(w.last) >= 1024 {
		for k, t := range w.last {
			if now.Sub(t) >= warnInterval {
				delete(w.last, k)
			}
		}
	}
	w.last[key] = now
	logWarn(msg, args...)
}

// reset forgets past warnings, so problems in a newly loaded config show at once.
func (w *warnLimiter) reset() {
	clear(w.last)
}
