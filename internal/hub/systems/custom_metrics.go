package systems

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	// retiredCustomMetricsMaxAge is how long the hub keeps the names of a custom
	// metric series after the agent stops reporting it: the longest chart range.
	retiredCustomMetricsMaxAge = 30 * 24 * time.Hour
	// maxRetiredCustomMetrics bounds the names kept per system, as the agent's
	// max_series ceiling bounds the series it reports. With both full,
	// systems.info holds metadata for 512 series: 256 in cmm and 256 in cmr.
	maxRetiredCustomMetrics = 256
)

// RetiredCustomMetric is the metadata of a custom metric series the agent no
// longer reports. The hub keeps it in systems.info as cmr, so the series'
// history keeps its title and unit while it is still shown.
type RetiredCustomMetric struct {
	system.CustomMetricMeta
	LastReported int64 `json:"t"` // Unix seconds, to within one update
}

// retainCustomMetrics returns the metadata to keep for custom metric series
// the agent reported before but not now. A series reported again leaves the
// list. One not reported for retiredCustomMetricsMaxAge, or beyond the newest
// maxRetiredCustomMetrics, is forgotten.
//
// Pausing a system clears its info, these names included (see the pause hook
// in system_manager.go), which is left as upstream has it: a paused system
// shows no custom history until its agent reports again.
func retainCustomMetrics(prevReported map[string]system.CustomMetricMeta, prevRetired map[string]RetiredCustomMetric,
	reported map[string]system.CustomMetricMeta, now time.Time) map[string]RetiredCustomMetric {
	retired := make(map[string]RetiredCustomMetric, len(prevRetired)+len(prevReported))
	oldest := now.Add(-retiredCustomMetricsMaxAge).Unix()
	for key, r := range prevRetired {
		if r.LastReported > oldest {
			retired[key] = r
		}
	}
	for key, meta := range prevReported {
		retired[key] = RetiredCustomMetric{CustomMetricMeta: meta, LastReported: now.Unix()}
	}
	for key := range reported {
		delete(retired, key)
	}
	if len(retired) > maxRetiredCustomMetrics {
		// newest first, then by key, so the same entries survive every time
		keys := slices.SortedFunc(maps.Keys(retired), func(a, b string) int {
			return cmp.Or(cmp.Compare(retired[b].LastReported, retired[a].LastReported), cmp.Compare(a, b))
		})
		for _, key := range keys[maxRetiredCustomMetrics:] {
			delete(retired, key)
		}
	}
	if len(retired) == 0 {
		return nil
	}
	return retired
}
