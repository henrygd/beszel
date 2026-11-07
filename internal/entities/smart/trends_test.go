package smart

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr(value uint64) *uint64 { return &value }

func TestAttribute5Thresholds(t *testing.T) {
	warningCases := []struct {
		name string
		m    SmartTrendMetric
		acc  bool
	}{
		{"raw", SmartTrendMetric{Raw: 50}, false},
		{"24h", SmartTrendMetric{Delta24h: ptr(2)}, false},
		{"7d", SmartTrendMetric{Delta7d: ptr(3)}, false},
		{"30d", SmartTrendMetric{Delta30d: ptr(5)}, false},
		{"90d", SmartTrendMetric{Delta90d: ptr(10)}, false},
		{"365d", SmartTrendMetric{Delta365d: ptr(20)}, false},
		{"consecutive", SmartTrendMetric{ConsecutiveIncreasing: 3}, false},
		{"double 30d", SmartTrendMetric{Delta30d: ptr(4), PreviousDelta30d: ptr(2)}, false},
		{"double 7d", SmartTrendMetric{Delta7d: ptr(2), PreviousDelta7d: ptr(1)}, false},
	}
	for _, tc := range warningCases {
		t.Run("warning "+tc.name, func(t *testing.T) {
			warning, _ := evaluateAttribute5(tc.m, tc.acc)
			assert.True(t, warning)
		})
	}

	criticalCases := []struct {
		name string
		m    SmartTrendMetric
		acc  bool
	}{
		{"24h", SmartTrendMetric{Delta24h: ptr(10)}, false},
		{"7d", SmartTrendMetric{Delta7d: ptr(20)}, false},
		{"30d", SmartTrendMetric{Delta30d: ptr(50)}, false},
		{"raw 500 growing", SmartTrendMetric{Raw: 500, Delta30d: ptr(1)}, false},
		{"consecutive", SmartTrendMetric{ConsecutiveIncreasing: 5}, false},
		{"triple 7d", SmartTrendMetric{Delta7d: ptr(6), PreviousDelta7d: ptr(2)}, false},
		{"accelerating", SmartTrendMetric{}, true},
	}
	for _, tc := range criticalCases {
		t.Run("critical "+tc.name, func(t *testing.T) {
			_, critical := evaluateAttribute5(tc.m, tc.acc)
			assert.True(t, critical)
		})
	}
}

func TestAttribute197Thresholds(t *testing.T) {
	warningCases := []struct {
		name string
		m    SmartTrendMetric
		re   bool
	}{
		{"persisted", SmartTrendMetric{Raw: 1, PersistedHours: 24}, false},
		{"raw", SmartTrendMetric{Raw: 5}, false},
		{"24h", SmartTrendMetric{Delta24h: ptr(1)}, false},
		{"7d", SmartTrendMetric{Delta7d: ptr(2)}, false},
		{"30d", SmartTrendMetric{Delta30d: ptr(3)}, false},
		{"nonzero samples", SmartTrendMetric{ConsecutiveNonzero: 2}, false},
		{"reappeared", SmartTrendMetric{}, true},
		{"increasing", SmartTrendMetric{ConsecutiveIncreasing: 2}, false},
	}
	for _, tc := range warningCases {
		t.Run("warning "+tc.name, func(t *testing.T) {
			warning, _ := evaluateAttribute197(tc.m, SmartTrendMetric{}, SmartTrendMetric{}, tc.re)
			assert.True(t, warning)
		})
	}

	criticalCases := []struct {
		name string
		m    SmartTrendMetric
		m5   SmartTrendMetric
		m198 SmartTrendMetric
	}{
		{"raw", SmartTrendMetric{Raw: 20}, SmartTrendMetric{}, SmartTrendMetric{}},
		{"24h", SmartTrendMetric{Delta24h: ptr(5)}, SmartTrendMetric{}, SmartTrendMetric{}},
		{"7d", SmartTrendMetric{Delta7d: ptr(10)}, SmartTrendMetric{}, SmartTrendMetric{}},
		{"persisted", SmartTrendMetric{Raw: 1, PersistedHours: 168}, SmartTrendMetric{}, SmartTrendMetric{}},
		{"increasing", SmartTrendMetric{ConsecutiveIncreasing: 3}, SmartTrendMetric{}, SmartTrendMetric{}},
		{"with 198", SmartTrendMetric{Raw: 5}, SmartTrendMetric{}, SmartTrendMetric{Raw: 1}},
		{"with growing 5", SmartTrendMetric{Raw: 5}, SmartTrendMetric{Delta30d: ptr(1)}, SmartTrendMetric{}},
	}
	for _, tc := range criticalCases {
		t.Run("critical "+tc.name, func(t *testing.T) {
			_, critical := evaluateAttribute197(tc.m, tc.m5, tc.m198, false)
			assert.True(t, critical)
		})
	}
}

func TestAttribute198ThresholdsAndStableClear(t *testing.T) {
	warningCases := []SmartTrendMetric{
		{Raw: 1},
		{Delta24h: ptr(1)},
		{Delta7d: ptr(1)},
		{ConsecutiveNonzero: 2},
	}
	for _, metric := range warningCases {
		warning, _ := evaluateAttribute198(metric, SmartTrendMetric{}, SmartTrendMetric{})
		assert.True(t, warning)
	}

	warning, _ := evaluateAttribute198(SmartTrendMetric{Raw: 1, Delta30d: ptr(0), ConsecutiveNonzero: 100}, SmartTrendMetric{}, SmartTrendMetric{})
	assert.False(t, warning, "a historical 198 count may clear after a stable 30d window")

	criticalCases := []struct {
		m    SmartTrendMetric
		m5   SmartTrendMetric
		m197 SmartTrendMetric
	}{
		{SmartTrendMetric{Raw: 10}, SmartTrendMetric{}, SmartTrendMetric{}},
		{SmartTrendMetric{Delta24h: ptr(2)}, SmartTrendMetric{}, SmartTrendMetric{}},
		{SmartTrendMetric{Delta7d: ptr(3)}, SmartTrendMetric{}, SmartTrendMetric{}},
		{SmartTrendMetric{ConsecutiveIncreasing: 2}, SmartTrendMetric{}, SmartTrendMetric{}},
		{SmartTrendMetric{Raw: 1}, SmartTrendMetric{}, SmartTrendMetric{Raw: 1}},
		{SmartTrendMetric{Raw: 1}, SmartTrendMetric{Delta30d: ptr(1)}, SmartTrendMetric{}},
	}
	for _, tc := range criticalCases {
		_, critical := evaluateAttribute198(tc.m, tc.m5, tc.m197)
		assert.True(t, critical)
	}
}

func TestCombinedRules(t *testing.T) {
	assert.True(t, combinedWarning(SmartTrendMetric{Raw: 1}, SmartTrendMetric{Raw: 1}, SmartTrendMetric{}, SmartTrendState{}))
	assert.True(t, combinedWarning(SmartTrendMetric{}, SmartTrendMetric{Raw: 1}, SmartTrendMetric{Raw: 1}, SmartTrendState{}))
	assert.True(t, combinedWarning(SmartTrendMetric{Delta30d: ptr(1)}, SmartTrendMetric{Delta30d: ptr(1)}, SmartTrendMetric{}, SmartTrendState{}))
	assert.True(t, combinedCritical(SmartTrendMetric{Delta30d: ptr(1)}, SmartTrendMetric{Raw: 1}, SmartTrendMetric{Raw: 1}, false))
	assert.True(t, combinedCritical(SmartTrendMetric{}, SmartTrendMetric{Raw: 5}, SmartTrendMetric{Raw: 1}, false))
	assert.True(t, combinedCritical(SmartTrendMetric{}, SmartTrendMetric{Delta24h: ptr(1)}, SmartTrendMetric{Delta24h: ptr(1)}, false))
	assert.True(t, combinedCritical(SmartTrendMetric{}, SmartTrendMetric{Raw: 1}, SmartTrendMetric{}, true))
}

func TestEvaluateSmartHealthStableSmallReallocatedCountIsPassed(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	attrs := []*SmartAttribute{{ID: 5, RawValue: 5}, {ID: 197}, {ID: 198}}
	state, health := EvaluateSmartHealth(now, "SERIAL", "PASSED", attrs, SmartTrendState{})
	assert.Equal(t, "PASSED", health.Status)

	state, health = EvaluateSmartHealth(now.Add(time.Hour), "SERIAL", "PASSED", attrs, state)
	assert.Equal(t, "PASSED", health.Status)
	assert.Len(t, state.Samples, 1, "unchanged samples should use change-point compression")
}

func TestEvaluateSmartHealthStableLargeReallocatedCountClearsAfterCleanWindows(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	attrs := []*SmartAttribute{{ID: 5, RawValue: 50}, {ID: 197}, {ID: 198}}
	state, health := EvaluateSmartHealth(now, "SERIAL", "PASSED", attrs, SmartTrendState{})
	assert.Equal(t, "WARNING", health.Status)

	_, health = EvaluateSmartHealth(now.Add(30*24*time.Hour), "SERIAL", "PASSED", attrs, state)
	assert.Equal(t, "PASSED", health.Status)
	assert.Empty(t, health.WarningReasons)
}

func TestEvaluateSmartHealthStableUncorrectableCountClearsAfterThirtyDays(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	attrs := []*SmartAttribute{{ID: 5}, {ID: 197}, {ID: 198, RawValue: 1}}
	state, health := EvaluateSmartHealth(now, "SERIAL", "PASSED", attrs, SmartTrendState{})
	assert.Equal(t, "WARNING", health.Status)

	_, health = EvaluateSmartHealth(now.Add(30*24*time.Hour), "SERIAL", "PASSED", attrs, state)
	assert.Equal(t, "PASSED", health.Status)
	assert.EqualValues(t, 1, health.Metrics[attrUncorrectable].Raw, "the historical raw value remains visible")
}

func TestEvaluateSmartHealthPendingPersistenceAndClear(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	pending := []*SmartAttribute{{ID: 5}, {ID: 197, RawValue: 1}, {ID: 198}}
	state, health := EvaluateSmartHealth(now, "SERIAL", "PASSED", pending, SmartTrendState{})
	assert.Equal(t, "PASSED", health.Status)

	state, health = EvaluateSmartHealth(now.Add(24*time.Hour), "SERIAL", "PASSED", pending, state)
	assert.Equal(t, "WARNING", health.Status)

	clear := []*SmartAttribute{{ID: 5}, {ID: 197}, {ID: 198}}
	state, health = EvaluateSmartHealth(now.Add(25*time.Hour), "SERIAL", "PASSED", clear, state)
	assert.Equal(t, "WARNING", health.Status, "warning must remain latched until 168 clean hours")

	_, health = EvaluateSmartHealth(now.Add(25*time.Hour+168*time.Hour), "SERIAL", "PASSED", clear, state)
	assert.Equal(t, "PASSED", health.Status)
}

func TestEvaluateSmartHealthResetsHistoryForReplacementSerial(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	state := SmartTrendState{Serial: "OLD", Samples: []TrendSample{{At: now.Add(-30 * 24 * time.Hour).Unix(), Reallocated: 100}}}
	attrs := []*SmartAttribute{{ID: 5, RawValue: 1}, {ID: 197}, {ID: 198}}
	state, health := EvaluateSmartHealth(now, "NEW", "PASSED", attrs, state)
	assert.Equal(t, "NEW", state.Serial)
	assert.Len(t, state.Samples, 1)
	assert.EqualValues(t, 1, state.Samples[0].Reallocated)
	assert.Equal(t, "PASSED", health.Status)
}

func TestEvaluateSmartHealthDoesNotAccumulateHistoryWithoutSerial(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	attrs := []*SmartAttribute{{ID: 5}, {ID: 197, RawValue: 1}, {ID: 198}}
	state, health := EvaluateSmartHealth(now, "", "PASSED", attrs, SmartTrendState{})
	assert.Equal(t, "PASSED", health.Status)

	state, health = EvaluateSmartHealth(now.Add(24*time.Hour), "", "PASSED", attrs, state)
	assert.Equal(t, "PASSED", health.Status)
	require.Len(t, state.Samples, 1)
	assert.Nil(t, health.Metrics[attrPending].Delta24h)
	assert.EqualValues(t, 1, health.Metrics[attrPending].ConsecutiveNonzero)
}

func TestCombinedWarningRemainsLatchedUntilAffectedAttributesAreClean(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	degraded := []*SmartAttribute{{ID: 5, RawValue: 1}, {ID: 197, RawValue: 1}, {ID: 198}}
	state, health := EvaluateSmartHealth(now, "SERIAL", "PASSED", degraded, SmartTrendState{})
	assert.Equal(t, "WARNING", health.Status)

	resolved := []*SmartAttribute{{ID: 5, RawValue: 1}, {ID: 197}, {ID: 198}}
	state, health = EvaluateSmartHealth(now.Add(time.Hour), "SERIAL", "PASSED", resolved, state)
	assert.Equal(t, "WARNING", health.Status)

	_, health = EvaluateSmartHealth(now.Add(30*24*time.Hour), "SERIAL", "PASSED", resolved, state)
	assert.Equal(t, "PASSED", health.Status)
}

func TestAcceleratingAcrossThreeSevenDayWindows(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	state := SmartTrendState{Samples: []TrendSample{
		{At: now.Add(-21 * 24 * time.Hour).Unix(), Reallocated: 0},
		{At: now.Add(-14 * 24 * time.Hour).Unix(), Reallocated: 1},
		{At: now.Add(-7 * 24 * time.Hour).Unix(), Reallocated: 3},
		{At: now.Unix(), Reallocated: 7},
	}}
	assert.True(t, acceleratingForThreeWindows(state, now, attrReallocated, 7*24*time.Hour))
}

func TestMissingHistoricalWindowsAreIgnoredAndNegativeDeltasClamp(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	state := SmartTrendState{Samples: []TrendSample{{At: now.Add(-8 * 24 * time.Hour).Unix(), Reallocated: 10}, {At: now.Unix(), Reallocated: 5}}}
	ensureTrendMaps(&state)
	metric := buildTrendMetric(state, now, attrReallocated)
	require.NotNil(t, metric.Delta7d)
	assert.Zero(t, *metric.Delta7d)
	assert.Nil(t, metric.Delta30d)
}

func TestReportedFailureAndUnknownTakePrecedence(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	attrs := []*SmartAttribute{{ID: 5, RawValue: 1000}, {ID: 197, RawValue: 100}, {ID: 198, RawValue: 100}}
	_, failed := EvaluateSmartHealth(now, "SERIAL", "FAILED", attrs, SmartTrendState{})
	_, unknown := EvaluateSmartHealth(now, "SERIAL", "UNKNOWN", attrs, SmartTrendState{})
	assert.Equal(t, "FAILED", failed.Status)
	assert.Equal(t, "UNKNOWN", unknown.Status)
}
