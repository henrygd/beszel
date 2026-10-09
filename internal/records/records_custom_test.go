//go:build testing

package records

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
)

func TestCustomMetricsAverage(t *testing.T) {
	tests := []struct {
		name  string
		input []map[string]float64
		want  map[string]float64
	}{
		{
			name:  "key in every record is a plain mean",
			input: []map[string]float64{{"power": 1.5}, {"power": 2.5}, {"power": 3.5}},
			want:  map[string]float64{"power": 2.5},
		},
		{
			name:  "keys appearing and disappearing mid-window",
			input: []map[string]float64{{"a": 1}, {"a": 3, "b": 10}, {"b": 20}, {}, {"a": 5, "c": 7}},
			want:  map[string]float64{"a": 3, "b": 15, "c": 7},
		},
		{
			name:  "differing key sets give the union",
			input: []map[string]float64{{"a": 2, "b": 4}, {"c": 6}, {"a": 4, "d": 8}},
			want:  map[string]float64{"a": 3, "b": 4, "c": 6, "d": 8},
		},
		{
			name:  "unrounded, so small ratios survive",
			input: []map[string]float64{{"ratio": 0.0012}, {"ratio": 0.0012}, {"ratio": 0.0012}},
			want:  map[string]float64{"ratio": 0.0012},
		},
		{
			name:  "negative values",
			input: []map[string]float64{{"offset": -2}, {"offset": -4}},
			want:  map[string]float64{"offset": -3},
		},
		{
			name:  "empty and nil maps",
			input: []map[string]float64{{}, nil, {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AverageCustomStatsSlice(tt.input)
			if tt.want == nil {
				assert.Nil(t, result, "no spurious keys")
				return
			}
			assert.InDeltaMapValues(t, tt.want, result, 1e-12)
			assert.Len(t, result, len(tt.want))
		})
	}
}

// The bug this guards against: dividing by the number of records instead of
// the records a key appears in reports a key seen in 3 of 60 records 20x too low.
func TestCustomMetricsAveragePerKeyCount(t *testing.T) {
	records := make([]map[string]float64, 60)
	for i := range records {
		records[i] = map[string]float64{"always": 1}
	}
	records[10]["sometimes"] = 2
	records[30]["sometimes"] = 4
	records[50]["sometimes"] = 6

	result := AverageCustomStatsSlice(records)
	assert.Equal(t, 4.0, result["sometimes"])
	assert.Equal(t, 1.0, result["always"])
}

// Summing values near MaxFloat64 overflows to +Inf, which JSON cannot encode,
// so the hub would fail to store the record. Only that key is dropped.
func TestCustomMetricsAverageDropsOverflow(t *testing.T) {
	records := []map[string]float64{
		{"big": math.MaxFloat64, "ok": 1},
		{"big": math.MaxFloat64, "ok": 3},
	}
	result := AverageCustomStatsSlice(records)
	assert.Equal(t, map[string]float64{"ok": 2}, result)

	_, err := json.Marshal(result)
	assert.NoError(t, err)
}

func TestCustomMetricsAverageDoesNotMutateInput(t *testing.T) {
	records := []map[string]float64{{"a": 1}, {"a": 3, "b": 5}}
	result := AverageCustomStatsSlice(records)
	result["a"] = 100

	assert.Equal(t, map[string]float64{"a": 1}, records[0])
	assert.Equal(t, map[string]float64{"a": 3, "b": 5}, records[1])
}

func TestCustomMetricsAverageNoRecords(t *testing.T) {
	assert.Nil(t, AverageCustomStatsSlice(nil))
	assert.Nil(t, AverageCustomStatsSlice([]map[string]float64{{}}))
}

// Values live in custom_stats, so averaging system_stats leaves them out even
// if a record still carries them.
func TestSystemStatsAverageLeavesOutCustomMetrics(t *testing.T) {
	records := []system.Stats{{Cpu: 10, CustomMetrics: map[string]float64{"a": 1}}}
	assert.Nil(t, AverageSystemStatsSlice(records).CustomMetrics)
}
