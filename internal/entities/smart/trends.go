package smart

import "time"

const smartTrendRetention = 366 * 24 * time.Hour

const (
	attrReallocated         uint16 = 5
	attrPending             uint16 = 197
	attrUncorrectable       uint16 = 198
	trendAccelerationWindow        = 7 * 24 * time.Hour
)

// TrendSample is a change-point snapshot of the three ATA media-error counters.
// Unchanged samples update streak metadata without growing the history array.
type TrendSample struct {
	At            int64  `json:"t"`
	Reallocated   uint64 `json:"5"`
	Pending       uint64 `json:"197"`
	Uncorrectable uint64 `json:"198"`
}

// SmartTrendState is persisted by the hub alongside a SMART device record.
type SmartTrendState struct {
	Serial                string          `json:"serial,omitempty"`
	Samples               []TrendSample   `json:"samples,omitempty"`
	LastSampleAt          int64           `json:"last_sample_at,omitempty"`
	ConsecutiveIncreasing map[uint16]uint `json:"consecutive_increasing,omitempty"`
	ConsecutiveNonzero    map[uint16]uint `json:"consecutive_nonzero,omitempty"`
	PendingSeenNonzero    bool            `json:"pending_seen_nonzero,omitempty"`
	PendingCleared        bool            `json:"pending_cleared,omitempty"`
	PendingReappeared     bool            `json:"pending_reappeared,omitempty"`
	Warned                map[uint16]bool `json:"warned,omitempty"`
	Status                string          `json:"status,omitempty"`
}

// SmartTrendMetric contains the current raw value and available historical deltas.
type SmartTrendMetric struct {
	Raw                   uint64  `json:"raw"`
	Delta24h              *uint64 `json:"delta_24h,omitempty"`
	Delta7d               *uint64 `json:"delta_7d,omitempty"`
	Delta30d              *uint64 `json:"delta_30d,omitempty"`
	Delta90d              *uint64 `json:"delta_90d,omitempty"`
	Delta365d             *uint64 `json:"delta_365d,omitempty"`
	PreviousDelta7d       *uint64 `json:"previous_delta_7d,omitempty"`
	PreviousDelta30d      *uint64 `json:"previous_delta_30d,omitempty"`
	ConsecutiveIncreasing uint    `json:"consecutive_increasing"`
	ConsecutiveNonzero    uint    `json:"consecutive_nonzero"`
	PersistedHours        uint64  `json:"persisted_hours,omitempty"`
	RemainedZeroHours     uint64  `json:"remained_zero_hours,omitempty"`
}

// SmartHealthAnalysis explains the hub-derived state shown to users.
type SmartHealthAnalysis struct {
	Status                     string                      `json:"status"`
	ReportedStatus             string                      `json:"reported_status"`
	WarningReasons             []string                    `json:"warning_reasons,omitempty"`
	CriticalReasons            []string                    `json:"critical_reasons,omitempty"`
	Metrics                    map[uint16]SmartTrendMetric `json:"metrics,omitempty"`
	AcceleratingFor3Windows    bool                        `json:"accelerating_for_3_windows,omitempty"`
	PendingClearedThenReturned bool                        `json:"pending_cleared_then_returned,omitempty"`
}

// EvaluateSmartHealth records a sample and applies trend-aware ATA health rules.
// Missing historical windows are deliberately ignored instead of treated as zero.
func EvaluateSmartHealth(now time.Time, serial, reportedStatus string, attributes []*SmartAttribute, prior SmartTrendState) (SmartTrendState, SmartHealthAnalysis) {
	values, hasTargetAttributes := criticalATAValues(attributes)
	hasPriorHistory := len(prior.Samples) > 0 || prior.LastSampleAt != 0
	if serial == "" || (hasPriorHistory && prior.Serial != serial) {
		prior = SmartTrendState{}
	}
	prior.Serial = serial
	ensureTrendMaps(&prior)

	analysis := SmartHealthAnalysis{ReportedStatus: reportedStatus}
	if !hasTargetAttributes {
		analysis.Status = reportedStatus
		prior.Status = analysis.Status
		return prior, analysis
	}

	previous := latestTrendValues(prior.Samples)
	updateTrendStreaks(&prior, previous, values)
	updatePendingTransitions(&prior, previous.Pending, values.Pending)
	appendTrendChangePoint(&prior, now, values)
	pruneTrendHistory(&prior, now)
	prior.LastSampleAt = now.Unix()

	metrics := map[uint16]SmartTrendMetric{
		attrReallocated:   buildTrendMetric(prior, now, attrReallocated),
		attrPending:       buildTrendMetric(prior, now, attrPending),
		attrUncorrectable: buildTrendMetric(prior, now, attrUncorrectable),
	}
	analysis.Metrics = metrics
	analysis.PendingClearedThenReturned = prior.PendingReappeared

	m5 := metrics[attrReallocated]
	m197 := metrics[attrPending]
	m198 := metrics[attrUncorrectable]
	accelerating := acceleratingForThreeWindows(prior, now, attrReallocated, trendAccelerationWindow)
	analysis.AcceleratingFor3Windows = accelerating

	warn5, crit5 := evaluateAttribute5(m5, accelerating)
	warn197, crit197 := evaluateAttribute197(m197, m5, m198, prior.PendingReappeared)
	warn198, crit198 := evaluateAttribute198(m198, m5, m197)

	clear5 := clearAttribute5Warning(m5, m197, m198)
	if clear5 && !attribute5DynamicWarning(m5) {
		warn5 = false
	}

	combinedWarn := combinedWarningAttributes(m5, m197, m198, prior)
	combinedCritical := combinedCriticalAttributes(m5, m197, m198, accelerating)
	warn5 = warn5 || combinedWarn.Reallocated || combinedCritical.Reallocated
	warn197 = warn197 || combinedWarn.Pending || combinedCritical.Pending
	warn198 = warn198 || combinedWarn.Uncorrectable || combinedCritical.Uncorrectable

	warn5 = applyWarningLatch(prior.Warned[attrReallocated], warn5, clear5, &analysis.WarningReasons, "attribute 5 waiting for clean 7d/30d windows")
	warn197 = applyWarningLatch(prior.Warned[attrPending], warn197, m197.Raw == 0 && m197.RemainedZeroHours >= 168, &analysis.WarningReasons, "attribute 197 waiting for 168 clean hours")
	warn198 = applyWarningLatch(prior.Warned[attrUncorrectable], warn198, deltaEquals(m198.Delta30d, 0), &analysis.WarningReasons, "attribute 198 waiting for a clean 30d window")

	analysis.WarningReasons = uniqueStrings(append(analysis.WarningReasons, warningReasons(m5, m197, m198, prior, accelerating)...))
	analysis.CriticalReasons = uniqueStrings(append(analysis.CriticalReasons, criticalReasons(m5, m197, m198, accelerating)...))

	isCritical := crit5 || crit197 || crit198 || combinedCritical.Any()
	isWarning := warn5 || warn197 || warn198

	prior.Warned[attrReallocated] = warn5 || crit5
	prior.Warned[attrPending] = warn197 || crit197
	prior.Warned[attrUncorrectable] = warn198 || crit198
	if m197.Raw == 0 && m197.RemainedZeroHours >= 168 {
		prior.PendingReappeared = false
	}

	switch reportedStatus {
	case "FAILED", "UNKNOWN":
		analysis.Status = reportedStatus
	case "PASSED", "WARNING":
		switch {
		case isCritical:
			analysis.Status = "CRITICAL"
		case isWarning:
			analysis.Status = "WARNING"
		default:
			analysis.Status = "PASSED"
		}
	default:
		analysis.Status = reportedStatus
	}
	prior.Status = analysis.Status
	return prior, analysis
}

type trendValues struct {
	Reallocated   uint64
	Pending       uint64
	Uncorrectable uint64
}

func criticalATAValues(attributes []*SmartAttribute) (trendValues, bool) {
	var values trendValues
	found := false
	for _, attr := range attributes {
		if attr == nil {
			continue
		}
		switch attr.ID {
		case attrReallocated:
			values.Reallocated = attr.RawValue
			found = true
		case attrPending:
			values.Pending = attr.RawValue
			found = true
		case attrUncorrectable:
			values.Uncorrectable = attr.RawValue
			found = true
		}
	}
	return values, found
}

func ensureTrendMaps(state *SmartTrendState) {
	if state.ConsecutiveIncreasing == nil {
		state.ConsecutiveIncreasing = make(map[uint16]uint)
	}
	if state.ConsecutiveNonzero == nil {
		state.ConsecutiveNonzero = make(map[uint16]uint)
	}
	if state.Warned == nil {
		state.Warned = make(map[uint16]bool)
	}
}

func latestTrendValues(samples []TrendSample) trendValues {
	if len(samples) == 0 {
		return trendValues{}
	}
	last := samples[len(samples)-1]
	return trendValues{last.Reallocated, last.Pending, last.Uncorrectable}
}

func updateTrendStreaks(state *SmartTrendState, previous, current trendValues) {
	for _, item := range []struct {
		id       uint16
		previous uint64
		current  uint64
	}{
		{attrReallocated, previous.Reallocated, current.Reallocated},
		{attrPending, previous.Pending, current.Pending},
		{attrUncorrectable, previous.Uncorrectable, current.Uncorrectable},
	} {
		if len(state.Samples) > 0 && item.current > item.previous {
			state.ConsecutiveIncreasing[item.id]++
		} else {
			state.ConsecutiveIncreasing[item.id] = 0
		}
		if item.current > 0 {
			state.ConsecutiveNonzero[item.id]++
		} else {
			state.ConsecutiveNonzero[item.id] = 0
		}
	}
}

func updatePendingTransitions(state *SmartTrendState, previous, current uint64) {
	if len(state.Samples) == 0 {
		state.PendingSeenNonzero = current > 0
		return
	}
	if previous > 0 && current == 0 {
		state.PendingCleared = true
	}
	if previous == 0 && current > 0 {
		if state.PendingSeenNonzero && state.PendingCleared {
			state.PendingReappeared = true
		}
		state.PendingSeenNonzero = true
	}
}

func appendTrendChangePoint(state *SmartTrendState, now time.Time, values trendValues) {
	if len(state.Samples) > 0 {
		last := state.Samples[len(state.Samples)-1]
		if last.Reallocated == values.Reallocated && last.Pending == values.Pending && last.Uncorrectable == values.Uncorrectable {
			return
		}
	}
	state.Samples = append(state.Samples, TrendSample{
		At:            now.Unix(),
		Reallocated:   values.Reallocated,
		Pending:       values.Pending,
		Uncorrectable: values.Uncorrectable,
	})
}

func pruneTrendHistory(state *SmartTrendState, now time.Time) {
	cutoff := now.Add(-smartTrendRetention).Unix()
	if len(state.Samples) < 2 || state.Samples[0].At >= cutoff {
		return
	}
	anchor := 0
	for i := 1; i < len(state.Samples) && state.Samples[i].At <= cutoff; i++ {
		anchor = i
	}
	state.Samples = append([]TrendSample(nil), state.Samples[anchor:]...)
}

func buildTrendMetric(state SmartTrendState, now time.Time, id uint16) SmartTrendMetric {
	current := valueForAttribute(latestTrendValues(state.Samples), id)
	metric := SmartTrendMetric{
		Raw:                   current,
		ConsecutiveIncreasing: state.ConsecutiveIncreasing[id],
		ConsecutiveNonzero:    state.ConsecutiveNonzero[id],
	}
	metric.Delta24h = deltaForWindow(state.Samples, now, id, 24*time.Hour)
	metric.Delta7d = deltaForWindow(state.Samples, now, id, 7*24*time.Hour)
	metric.Delta30d = deltaForWindow(state.Samples, now, id, 30*24*time.Hour)
	metric.Delta90d = deltaForWindow(state.Samples, now, id, 90*24*time.Hour)
	metric.Delta365d = deltaForWindow(state.Samples, now, id, 365*24*time.Hour)
	metric.PreviousDelta7d = deltaBetween(state.Samples, now, id, 7*24*time.Hour, 14*24*time.Hour)
	metric.PreviousDelta30d = deltaBetween(state.Samples, now, id, 30*24*time.Hour, 60*24*time.Hour)
	if current > 0 {
		metric.PersistedHours = contiguousHours(state.Samples, now, id, true)
	} else {
		metric.RemainedZeroHours = contiguousHours(state.Samples, now, id, false)
	}
	return metric
}

func deltaForWindow(samples []TrendSample, now time.Time, id uint16, window time.Duration) *uint64 {
	current, ok := valueAt(samples, now.Unix(), id)
	if !ok {
		return nil
	}
	previous, ok := valueAt(samples, now.Add(-window).Unix(), id)
	if !ok {
		return nil
	}
	value := clampedDelta(current, previous)
	return &value
}

func deltaBetween(samples []TrendSample, now time.Time, id uint16, newerAgo, olderAgo time.Duration) *uint64 {
	newer, ok := valueAt(samples, now.Add(-newerAgo).Unix(), id)
	if !ok {
		return nil
	}
	older, ok := valueAt(samples, now.Add(-olderAgo).Unix(), id)
	if !ok {
		return nil
	}
	value := clampedDelta(newer, older)
	return &value
}

func valueAt(samples []TrendSample, at int64, id uint16) (uint64, bool) {
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].At <= at {
			return valueForSample(samples[i], id), true
		}
	}
	return 0, false
}

func valueForSample(sample TrendSample, id uint16) uint64 {
	return valueForAttribute(trendValues{sample.Reallocated, sample.Pending, sample.Uncorrectable}, id)
}

func valueForAttribute(values trendValues, id uint16) uint64 {
	switch id {
	case attrReallocated:
		return values.Reallocated
	case attrPending:
		return values.Pending
	case attrUncorrectable:
		return values.Uncorrectable
	default:
		return 0
	}
}

func clampedDelta(current, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}

func contiguousHours(samples []TrendSample, now time.Time, id uint16, nonzero bool) uint64 {
	if len(samples) == 0 {
		return 0
	}
	start := samples[len(samples)-1].At
	for i := len(samples) - 1; i >= 0; i-- {
		matches := valueForSample(samples[i], id) > 0
		if matches != nonzero {
			break
		}
		start = samples[i].At
	}
	if start > now.Unix() {
		return 0
	}
	return uint64(now.Unix()-start) / 3600
}

func acceleratingForThreeWindows(state SmartTrendState, now time.Time, id uint16, window time.Duration) bool {
	current := deltaForWindow(state.Samples, now, id, window)
	previous := deltaBetween(state.Samples, now, id, window, 2*window)
	before := deltaBetween(state.Samples, now, id, 2*window, 3*window)
	return current != nil && previous != nil && before != nil && *current > *previous && *previous > *before && *current > 0
}

func evaluateAttribute5(m SmartTrendMetric, accelerating bool) (warning, critical bool) {
	warning = m.Raw >= 50 || attribute5DynamicWarning(m)
	critical = atLeast(m.Delta24h, 10) || atLeast(m.Delta7d, 20) || atLeast(m.Delta30d, 50) || (m.Raw >= 500 && positive(m.Delta30d)) || m.ConsecutiveIncreasing >= 5 || multipleIncrease(m.Delta7d, m.PreviousDelta7d, 3, 5) || accelerating
	return
}

func attribute5DynamicWarning(m SmartTrendMetric) bool {
	return atLeast(m.Delta24h, 2) || atLeast(m.Delta7d, 3) || atLeast(m.Delta30d, 5) || atLeast(m.Delta90d, 10) || atLeast(m.Delta365d, 20) || m.ConsecutiveIncreasing >= 3 || multipleIncrease(m.Delta30d, m.PreviousDelta30d, 2, 3) || multipleIncrease(m.Delta7d, m.PreviousDelta7d, 2, 2)
}

func evaluateAttribute197(m, m5, m198 SmartTrendMetric, reappeared bool) (warning, critical bool) {
	warning = (m.Raw >= 1 && m.PersistedHours >= 24) || m.Raw >= 5 || atLeast(m.Delta24h, 1) || atLeast(m.Delta7d, 2) || atLeast(m.Delta30d, 3) || m.ConsecutiveNonzero >= 2 || reappeared || m.ConsecutiveIncreasing >= 2
	critical = m.Raw >= 20 || atLeast(m.Delta24h, 5) || atLeast(m.Delta7d, 10) || (m.Raw > 0 && m.PersistedHours >= 168) || m.ConsecutiveIncreasing >= 3 || (m.Raw >= 5 && m198.Raw > 0) || (m.Raw >= 5 && positive(m5.Delta30d))
	return
}

func evaluateAttribute198(m, m5, m197 SmartTrendMetric) (warning, critical bool) {
	stableHistoricalCount := m.Raw > 0 && deltaEquals(m.Delta30d, 0)
	warning = (!stableHistoricalCount && (m.Raw >= 1 || m.ConsecutiveNonzero >= 2)) || atLeast(m.Delta24h, 1) || atLeast(m.Delta7d, 1)
	critical = m.Raw >= 10 || atLeast(m.Delta24h, 2) || atLeast(m.Delta7d, 3) || m.ConsecutiveIncreasing >= 2 || (m.Raw > 0 && m197.Raw > 0) || (m.Raw > 0 && positive(m5.Delta30d))
	return
}

type affectedAttributes struct {
	Reallocated   bool
	Pending       bool
	Uncorrectable bool
}

func (affected affectedAttributes) Any() bool {
	return affected.Reallocated || affected.Pending || affected.Uncorrectable
}

func combinedWarningAttributes(m5, m197, m198 SmartTrendMetric, state SmartTrendState) affectedAttributes {
	var affected affectedAttributes
	mark := func(condition, reallocated, pending, uncorrectable bool) {
		if !condition {
			return
		}
		affected.Reallocated = affected.Reallocated || reallocated
		affected.Pending = affected.Pending || pending
		affected.Uncorrectable = affected.Uncorrectable || uncorrectable
	}
	mark(atLeast(m5.Delta30d, 5), true, false, false)
	mark(m197.Raw >= 1 && m197.PersistedHours >= 24, false, true, false)
	mark(m198.Raw >= 1 && !deltaEquals(m198.Delta30d, 0), false, false, true)
	mark(m5.Raw > 0 && m197.Raw > 0, true, true, false)
	mark(m5.Raw > 0 && m198.Raw > 0, true, false, true)
	mark(m197.Raw > 0 && m198.Raw > 0, false, true, true)
	mark(m5.ConsecutiveIncreasing >= 3, true, false, false)
	mark(state.PendingReappeared, false, true, false)
	mark(positive(m5.Delta30d) && positive(m197.Delta30d), true, true, false)
	return affected
}

func combinedCriticalAttributes(m5, m197, m198 SmartTrendMetric, accelerating bool) affectedAttributes {
	var affected affectedAttributes
	mark := func(condition, reallocated, pending, uncorrectable bool) {
		if !condition {
			return
		}
		affected.Reallocated = affected.Reallocated || reallocated
		affected.Pending = affected.Pending || pending
		affected.Uncorrectable = affected.Uncorrectable || uncorrectable
	}
	mark(m197.Raw >= 20, false, true, false)
	mark(m198.Raw >= 10, false, false, true)
	mark(atLeast(m5.Delta7d, 20), true, false, false)
	mark(atLeast(m5.Delta30d, 50), true, false, false)
	mark(positive(m5.Delta30d) && m197.Raw > 0 && m198.Raw > 0, true, true, true)
	mark(positive(m5.Delta7d) && positive(m197.Delta7d) && positive(m198.Delta7d), true, true, true)
	mark(m197.Raw >= 5 && m198.Raw >= 1, false, true, true)
	mark(atLeast(m197.Delta24h, 1) && atLeast(m198.Delta24h, 1), false, true, true)
	mark(accelerating && (m197.Raw > 0 || m198.Raw > 0), true, m197.Raw > 0, m198.Raw > 0)
	return affected
}

func combinedWarning(m5, m197, m198 SmartTrendMetric, state SmartTrendState) bool {
	return combinedWarningAttributes(m5, m197, m198, state).Any()
}

func combinedCritical(m5, m197, m198 SmartTrendMetric, accelerating bool) bool {
	return combinedCriticalAttributes(m5, m197, m198, accelerating).Any()
}

func clearAttribute5Warning(m5, m197, m198 SmartTrendMetric) bool {
	return deltaEquals(m5.Delta30d, 0) && deltaEquals(m5.Delta7d, 0) && m197.Raw == 0 && deltaEquals(m198.Delta30d, 0)
}

func applyWarningLatch(wasWarned, active, canClear bool, reasons *[]string, reason string) bool {
	if active || !wasWarned || canClear {
		return active
	}
	*reasons = append(*reasons, reason)
	return true
}

func warningReasons(m5, m197, m198 SmartTrendMetric, state SmartTrendState, accelerating bool) []string {
	reasons := make([]string, 0, 24)
	add := func(ok bool, reason string) {
		if ok {
			reasons = append(reasons, reason)
		}
	}
	add(m5.Raw >= 50 && !clearAttribute5Warning(m5, m197, m198), "attribute 5 raw value is at least 50")
	add(atLeast(m5.Delta24h, 2), "attribute 5 increased by at least 2 in 24h")
	add(atLeast(m5.Delta7d, 3), "attribute 5 increased by at least 3 in 7d")
	add(atLeast(m5.Delta30d, 5), "attribute 5 increased by at least 5 in 30d")
	add(atLeast(m5.Delta90d, 10), "attribute 5 increased by at least 10 in 90d")
	add(atLeast(m5.Delta365d, 20), "attribute 5 increased by at least 20 in 365d")
	add(m5.ConsecutiveIncreasing >= 3, "attribute 5 increased in at least 3 consecutive samples")
	add(multipleIncrease(m5.Delta30d, m5.PreviousDelta30d, 2, 3), "attribute 5 30d growth at least doubled and reached 3")
	add(multipleIncrease(m5.Delta7d, m5.PreviousDelta7d, 2, 2), "attribute 5 7d growth at least doubled and reached 2")
	add(m197.Raw >= 5, "attribute 197 raw value is at least 5")
	add(m197.Raw > 0 && m197.PersistedHours >= 24, "attribute 197 remained nonzero for at least 24h")
	add(atLeast(m197.Delta24h, 1), "attribute 197 increased within 24h")
	add(atLeast(m197.Delta7d, 2), "attribute 197 increased by at least 2 in 7d")
	add(atLeast(m197.Delta30d, 3), "attribute 197 increased by at least 3 in 30d")
	add(m197.ConsecutiveNonzero >= 2, "attribute 197 was nonzero in at least 2 consecutive samples")
	add(m197.ConsecutiveIncreasing >= 2, "attribute 197 increased in at least 2 consecutive samples")
	add(state.PendingReappeared, "attribute 197 cleared and reappeared")
	add(m198.Raw > 0 && !deltaEquals(m198.Delta30d, 0), "attribute 198 is nonzero without a clean 30d window")
	add(atLeast(m198.Delta24h, 1), "attribute 198 increased within 24h")
	add(atLeast(m198.Delta7d, 1), "attribute 198 increased within 7d")
	add(m198.ConsecutiveNonzero >= 2 && !deltaEquals(m198.Delta30d, 0), "attribute 198 was nonzero in at least 2 consecutive samples")
	add(m5.Raw > 0 && m197.Raw > 0, "attributes 5 and 197 are both nonzero")
	add(m5.Raw > 0 && m198.Raw > 0, "attributes 5 and 198 are both nonzero")
	add(m197.Raw > 0 && m198.Raw > 0, "attributes 197 and 198 are both nonzero")
	add(positive(m5.Delta30d) && positive(m197.Delta30d), "attributes 5 and 197 both increased within 30d")
	add(accelerating, "attribute 5 is accelerating across three 7d windows")
	return reasons
}

func criticalReasons(m5, m197, m198 SmartTrendMetric, accelerating bool) []string {
	reasons := make([]string, 0, 20)
	add := func(ok bool, reason string) {
		if ok {
			reasons = append(reasons, reason)
		}
	}
	add(atLeast(m5.Delta24h, 10), "attribute 5 increased by at least 10 in 24h")
	add(atLeast(m5.Delta7d, 20), "attribute 5 increased by at least 20 in 7d")
	add(atLeast(m5.Delta30d, 50), "attribute 5 increased by at least 50 in 30d")
	add(m5.Raw >= 500 && positive(m5.Delta30d), "attribute 5 is at least 500 and still growing")
	add(m5.ConsecutiveIncreasing >= 5, "attribute 5 increased in at least 5 consecutive samples")
	add(multipleIncrease(m5.Delta7d, m5.PreviousDelta7d, 3, 5), "attribute 5 7d growth at least tripled and reached 5")
	add(accelerating, "attribute 5 is accelerating across three 7d windows")
	add(m197.Raw >= 20, "attribute 197 raw value is at least 20")
	add(atLeast(m197.Delta24h, 5), "attribute 197 increased by at least 5 in 24h")
	add(atLeast(m197.Delta7d, 10), "attribute 197 increased by at least 10 in 7d")
	add(m197.Raw > 0 && m197.PersistedHours >= 168, "attribute 197 remained nonzero for at least 168h")
	add(m197.ConsecutiveIncreasing >= 3, "attribute 197 increased in at least 3 consecutive samples")
	add(m197.Raw >= 5 && m198.Raw > 0, "attribute 197 is at least 5 while attribute 198 is nonzero")
	add(m197.Raw >= 5 && positive(m5.Delta30d), "attribute 197 is at least 5 while attribute 5 is growing")
	add(m198.Raw >= 10, "attribute 198 raw value is at least 10")
	add(atLeast(m198.Delta24h, 2), "attribute 198 increased by at least 2 in 24h")
	add(atLeast(m198.Delta7d, 3), "attribute 198 increased by at least 3 in 7d")
	add(m198.ConsecutiveIncreasing >= 2, "attribute 198 increased in at least 2 consecutive samples")
	add(m198.Raw > 0 && m197.Raw > 0, "attributes 197 and 198 are both nonzero")
	add(m198.Raw > 0 && positive(m5.Delta30d), "attribute 198 is nonzero while attribute 5 is growing")
	add(positive(m5.Delta30d) && m197.Raw > 0 && m198.Raw > 0, "attributes 5, 197, and 198 are jointly deteriorating")
	add(positive(m5.Delta7d) && positive(m197.Delta7d) && positive(m198.Delta7d), "attributes 5, 197, and 198 all increased within 7d")
	add(atLeast(m197.Delta24h, 1) && atLeast(m198.Delta24h, 1), "attributes 197 and 198 both increased within 24h")
	add(accelerating && (m197.Raw > 0 || m198.Raw > 0), "attribute 5 is accelerating with pending or uncorrectable sectors")
	return reasons
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := values[:0]
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func atLeast(value *uint64, threshold uint64) bool {
	return value != nil && *value >= threshold
}

func positive(value *uint64) bool {
	return value != nil && *value > 0
}

func deltaEquals(value *uint64, expected uint64) bool {
	return value != nil && *value == expected
}

func multipleIncrease(current, previous *uint64, multiplier, minimum uint64) bool {
	return current != nil && previous != nil && *current >= minimum && *current >= *previous*multiplier
}
