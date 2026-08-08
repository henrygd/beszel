package alerts

import (
	"math"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
)

func TestSelectTemperatureValue(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		thresholds, temperatures   map[string]float64
		fallback, defaultThreshold float64
		sensor                     string
		value, threshold           float64
		ok                         bool
	}{
		{name: "hottest not dashboard", temperatures: map[string]float64{"cpu": 85, "disk": 65}, fallback: 65, defaultThreshold: 80, sensor: "cpu", value: 85, threshold: 80, ok: true},
		{name: "individual margin", thresholds: map[string]float64{"cpu": 90, "disk": 60}, temperatures: map[string]float64{"cpu": 89, "disk": 61, "gpu": 100}, defaultThreshold: 80, sensor: "disk", value: 61, threshold: 60, ok: true},
		{name: "deterministic tie", thresholds: map[string]float64{"b": 70, "a": 60}, temperatures: map[string]float64{"b": 75, "a": 65}, sensor: "a", value: 65, threshold: 60, ok: true},
		{name: "legacy fallback", fallback: 82, defaultThreshold: 80, value: 82, threshold: 80, ok: true},
		{name: "empty selection fallback", thresholds: map[string]float64{}, fallback: 82, defaultThreshold: 80, value: 82, threshold: 80, ok: true},
		{name: "missing selected never fallback", thresholds: map[string]float64{"cpu": 80}, temperatures: map[string]float64{"gpu": 100}, fallback: 100, defaultThreshold: 80},
		{name: "partial selection cannot recover", thresholds: map[string]float64{"cpu": 80, "nvme": 60}, temperatures: map[string]float64{"cpu": 50}},
		{name: "partial selection at threshold cannot recover", thresholds: map[string]float64{"cpu": 80, "nvme": 60}, temperatures: map[string]float64{"cpu": 80}},
		{name: "invalid selected reading cannot recover", thresholds: map[string]float64{"cpu": 80, "nvme": 60}, temperatures: map[string]float64{"cpu": 50, "nvme": math.NaN()}},
		{name: "invalid selected threshold cannot recover", thresholds: map[string]float64{"cpu": 80, "nvme": math.NaN()}, temperatures: map[string]float64{"cpu": 50, "nvme": 50}},
		{name: "partial selection can trigger", thresholds: map[string]float64{"cpu": 80, "nvme": 60}, temperatures: map[string]float64{"nvme": 70}, sensor: "nvme", value: 70, threshold: 60, ok: true},
		{name: "complete selection can recover", thresholds: map[string]float64{"cpu": 80, "nvme": 60}, temperatures: map[string]float64{"cpu": 50, "nvme": 50}, sensor: "nvme", value: 50, threshold: 60, ok: true},
		{name: "invalid readings", temperatures: map[string]float64{"nan": math.NaN(), "inf": math.Inf(1), "zero": 0, "negative": -1}, defaultThreshold: 80},
		{name: "invalid threshold", thresholds: map[string]float64{"cpu": math.NaN()}, temperatures: map[string]float64{"cpu": 100}, fallback: 100, defaultThreshold: 80},
		{name: "infinite fallback", fallback: math.Inf(1), defaultThreshold: 80},
		{name: "invalid default", temperatures: map[string]float64{"cpu": 80}, fallback: 80, defaultThreshold: math.NaN()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			value, threshold, _, sensor, ok := selectTemperatureValue(tt.thresholds, tt.temperatures, tt.fallback, tt.defaultThreshold)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.sensor, sensor)
			assert.Equal(t, tt.value, value)
			assert.Equal(t, tt.threshold, threshold)
		})
	}
}

func TestTemperatureCacheRepopulate(t *testing.T) {
	collection := core.NewBaseCollection("alerts")
	collection.Fields.Add(&core.JSONField{Name: "thresholds"})
	record := core.NewRecord(collection)
	var cached CachedAlertData
	record.Set("thresholds", map[string]float64{"old": 80})
	cached.PopulateFromRecord(record)
	assert.Equal(t, map[string]float64{"old": 80}, cached.Thresholds)
	record.Set("thresholds", map[string]float64{"new": 70})
	cached.PopulateFromRecord(record)
	assert.Equal(t, map[string]float64{"new": 70}, cached.Thresholds)
	record.Set("thresholds", map[string]any{"bad": "not a number"})
	cached.PopulateFromRecord(record)
	assert.Nil(t, cached.Thresholds)
	record.Set("thresholds", nil)
	cached.PopulateFromRecord(record)
	assert.Nil(t, cached.Thresholds)
}
