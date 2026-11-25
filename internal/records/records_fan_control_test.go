//go:build testing

package records_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/records"
	"github.com/stretchr/testify/assert"
)

func TestAverageFanControl(t *testing.T) {
	for _, input := range [][]system.Stats{nil, {{}, {FanControl: map[string]float64{}}}} {
		assert.Nil(t, records.AverageSystemStatsSlice(input).FanControl)
	}
	input := []system.Stats{
		{}, // old agent or unavailable readings
		{FanControl: map[string]float64{"pwm": 40, "binary": 0, "off": 0}},
		{FanControl: map[string]float64{"pwm": 60, "binary": 100, "off": 0}},
		{FanControl: map[string]float64{"pwm": 80, "one-sample": 80}},
	}
	first := records.AverageSystemStatsSlice(input)
	assert.Equal(t, map[string]float64{"pwm": 60, "binary": 50, "off": 0, "one-sample": 80}, first.FanControl)
	second := records.AverageSystemStatsSlice([]system.Stats{first, {FanControl: map[string]float64{"pwm": 40, "binary": 100}}})
	assert.Equal(t, map[string]float64{"pwm": 50, "binary": 75, "off": 0, "one-sample": 80}, second.FanControl)
	rounded := records.AverageSystemStatsSlice([]system.Stats{
		{FanControl: map[string]float64{"pwm": 0}},
		{FanControl: map[string]float64{"pwm": 100}},
		{FanControl: map[string]float64{"pwm": 100}},
	})
	assert.Equal(t, 66.67, rounded.FanControl["pwm"])
}
