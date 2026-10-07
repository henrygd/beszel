//go:build testing

package systems

import (
	"fmt"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
)

func TestRetainCustomMetrics(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	board := system.CustomMetricMeta{Unit: "watts", DisplayName: "Board power", Chart: "Power consumption"}
	wall := system.CustomMetricMeta{Unit: "watts", DisplayName: "Wall power", Chart: "Power consumption"}
	retired := func(meta system.CustomMetricMeta, age time.Duration) RetiredCustomMetric {
		return RetiredCustomMetric{CustomMetricMeta: meta, LastReported: now.Add(-age).Unix()}
	}
	type metas = map[string]system.CustomMetricMeta
	type retirees = map[string]RetiredCustomMetric

	tests := []struct {
		name         string
		prevReported metas
		prevRetired  retirees
		reported     metas
		want         retirees
	}{
		{name: "nothing reported, nothing kept"},
		{
			name:         "still reported, nothing kept",
			prevReported: metas{"board": board},
			reported:     metas{"board": board},
		},
		{
			name:         "no longer reported: kept with the time it went",
			prevReported: metas{"board": board, "wall": wall},
			reported:     metas{"board": board},
			want:         retirees{"wall": retired(wall, 0)},
		},
		{
			name:         "all gone, as when the config is deleted",
			prevReported: metas{"board": board, "wall": wall},
			want:         retirees{"board": retired(board, 0), "wall": retired(wall, 0)},
		},
		{
			name:        "kept until 30 days after it went",
			prevRetired: retirees{"wall": retired(wall, 30*24*time.Hour-time.Second)},
			want:        retirees{"wall": retired(wall, 30*24*time.Hour-time.Second)},
		},
		{
			name:        "forgotten 30 days after it went",
			prevRetired: retirees{"wall": retired(wall, 30*24*time.Hour)},
		},
		{
			name:        "reported again: forgotten, the agent's metadata takes over",
			prevRetired: retirees{"wall": retired(wall, time.Hour)},
			reported:    metas{"wall": wall},
		},
		{
			name:         "the latest metadata replaces what was kept",
			prevReported: metas{"wall": wall},
			prevRetired:  retirees{"wall": retired(system.CustomMetricMeta{DisplayName: "old"}, time.Hour)},
			want:         retirees{"wall": retired(wall, 0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, retainCustomMetrics(tt.prevReported, tt.prevRetired, tt.reported, now))
		})
	}
}

// Label values that change over time, such as a date, can retire a new series
// every minute, so the kept names are capped, newest first.
func TestRetainCustomMetricsCap(t *testing.T) {
	now := time.Unix(1_790_000_000, 0)
	prev := make(map[string]RetiredCustomMetric)
	for i := range maxRetiredCustomMetrics + 44 {
		prev[fmt.Sprintf("k%03d", i)] = RetiredCustomMetric{LastReported: now.Add(-time.Duration(i) * time.Minute).Unix()}
	}
	got := retainCustomMetrics(nil, prev, nil, now)
	assert.Len(t, got, maxRetiredCustomMetrics)
	assert.Contains(t, got, "k000", "the newest is kept")
	assert.Contains(t, got, fmt.Sprintf("k%03d", maxRetiredCustomMetrics-1))
	assert.NotContains(t, got, fmt.Sprintf("k%03d", maxRetiredCustomMetrics), "the oldest are dropped")

	// Equal times keep the same entries every time.
	for key := range prev {
		prev[key] = RetiredCustomMetric{LastReported: now.Unix()}
	}
	first := retainCustomMetrics(nil, prev, nil, now)
	assert.Equal(t, first, retainCustomMetrics(nil, prev, nil, now))
	assert.Contains(t, first, "k000")
	assert.NotContains(t, first, fmt.Sprintf("k%03d", maxRetiredCustomMetrics+43))
}
