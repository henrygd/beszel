//go:build testing

package records_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/records"
	"github.com/stretchr/testify/assert"
)

func TestAverageSystemStatsSlice_CPUFrequencies(t *testing.T) {
	tests := []struct {
		name  string
		input []system.Stats
		want  []float64
	}{
		{"no records", nil, nil},
		{"unsupported agents", []system.Stats{{}, {CpuFreqs: []float64{}}}, nil},
		{"single sample", []system.Stats{{CpuFreqs: []float64{1.8, 2.4}}}, []float64{1.8, 2.4}},
		{"per-core average", []system.Stats{
			{CpuFreqs: []float64{1.2, 2.4}},
			{CpuFreqs: []float64{1.8, 3.6}},
		}, []float64{1.5, 3}},
		{"missing samples and different core counts", []system.Stats{
			{},
			{CpuFreqs: []float64{1.2}},
			{CpuFreqs: []float64{1.8, 2.4}},
			{CpuFreqs: []float64{2.4, 3.6, 1.5}},
			{CpuFreqs: []float64{}},
		}, []float64{1.8, 3, 1.5}},
		{"round to two decimals", []system.Stats{
			{CpuFreqs: []float64{1.21}},
			{CpuFreqs: []float64{1.22}},
			{CpuFreqs: []float64{1.22}},
		}, []float64{1.22}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := records.AverageSystemStatsSlice(tt.input)
			assert.Equal(t, tt.want, result.CpuFreqs)
		})
	}
}

func TestAverageSystemStatsSlice_CPUFrequencySuccessiveRollups(t *testing.T) {
	first := records.AverageSystemStatsSlice([]system.Stats{
		{CpuFreqs: []float64{1.2, 2.4}},
		{CpuFreqs: []float64{1.8, 3.6}},
	})
	second := records.AverageSystemStatsSlice([]system.Stats{
		{CpuFreqs: []float64{2.4, 3.6}},
		{CpuFreqs: []float64{3.0, 4.8}},
	})
	result := records.AverageSystemStatsSlice([]system.Stats{first, second})
	assert.Equal(t, []float64{2.1, 3.6}, result.CpuFreqs)
}
