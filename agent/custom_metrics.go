package agent

import (
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

// updateCustomMetrics sets custom metric values on the stats and their
// metadata on the system info. Both are nil, and so omitted from the
// response, when no metrics are configured.
func (a *Agent) updateCustomMetrics(cacheTimeMs uint16, systemStats *system.Stats) {
	if a.customMetrics == nil {
		a.systemInfo.CustomMetricsMeta = nil
		return
	}
	systemStats.CustomMetrics, a.systemInfo.CustomMetricsMeta = a.customMetrics.Collect(cacheTimeMs, time.Now())
}
