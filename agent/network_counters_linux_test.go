//go:build linux && testing

package agent

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEthtoolCounterCombinesHighWord(t *testing.T) {
	stats := map[string]uint64{
		"counter":   123,
		"counter_h": 2,
	}

	value, ok := ethtoolCounter(stats, "counter", "counter_h")
	require.True(t, ok)
	assert.Equal(t, uint64(2<<32+123), value)
}

func TestEthtoolCounterAddsLowWordAbove32Bits(t *testing.T) {
	// nvethernet accumulates each register in 64-bit software fields, so the low
	// word can carry past 32 bits; OR would drop the overlapping bit.
	stats := map[string]uint64{
		"counter":   1<<32 + 5,
		"counter_h": 1,
	}

	value, ok := ethtoolCounter(stats, "counter", "counter_h")
	require.True(t, ok)
	assert.Equal(t, uint64(2<<32+5), value)
}

func TestEthtoolCounterRejectsOverflow(t *testing.T) {
	stats := map[string]uint64{
		"counter":   math.MaxUint64,
		"counter_h": 1,
	}

	_, ok := ethtoolCounter(stats, "counter", "counter_h")
	assert.False(t, ok)
}

func TestEthtoolCounterFallsBackToLowWord(t *testing.T) {
	stats := map[string]uint64{"counter": 456}

	value, ok := ethtoolCounter(stats, "counter", "counter_h")
	require.True(t, ok)
	assert.Equal(t, uint64(456), value)
}
