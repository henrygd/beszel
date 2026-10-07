//go:build testing

package custommetrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWarnLimiter(t *testing.T) {
	logged := captureWarnings(t)
	w := newWarnLimiter()
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	w.warn(start, "parse:/a.prom", "Bad line", "file", "/a.prom")
	w.warn(start.Add(time.Minute), "parse:/a.prom", "Bad line", "file", "/a.prom")
	w.warn(start.Add(time.Minute), "parse:/b.prom", "Bad line", "file", "/b.prom")
	assert.Equal(t, []string{"Bad line file /a.prom", "Bad line file /b.prom"}, *logged)

	w.warn(start.Add(59*time.Minute), "parse:/a.prom", "Bad line", "file", "/a.prom")
	assert.Len(t, *logged, 2, "quiet within the hour")
	w.warn(start.Add(time.Hour), "parse:/a.prom", "Bad line", "file", "/a.prom")
	assert.Len(t, *logged, 3, "logged again after an hour")

	w.reset()
	w.warn(start.Add(time.Hour+time.Second), "parse:/b.prom", "Bad line", "file", "/b.prom")
	assert.Len(t, *logged, 4, "reset makes every warning show again")
}

func TestWarnLimiterPrunesExpiredKeys(t *testing.T) {
	captureWarnings(t)
	w := newWarnLimiter()
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for i := range 1024 {
		w.warn(start, fmt.Sprintf("k%d", i), "m")
	}
	w.warn(start.Add(2*time.Hour), "new", "m")
	assert.Len(t, w.last, 1)
}
