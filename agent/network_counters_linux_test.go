//go:build linux && testing

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEthtoolStats(t *testing.T) {
	stats := parseEthtoolStats(`
NIC statistics:
     mmc_tx_octetcount_gb: 4699696
     mmc_rx_octetcount_gb: 101028343
     ignored_text: not-a-number
     with_suffix: 42 packets
`)

	assert.Equal(t, uint64(4_699_696), stats["mmc_tx_octetcount_gb"])
	assert.Equal(t, uint64(101_028_343), stats["mmc_rx_octetcount_gb"])
	assert.Equal(t, uint64(42), stats["with_suffix"])
	assert.NotContains(t, stats, "ignored_text")
}

func TestEthtoolCounterCombinesHighWord(t *testing.T) {
	stats := map[string]uint64{
		"counter":   123,
		"counter_h": 2,
	}

	value, ok := ethtoolCounter(stats, "counter", "counter_h")
	require.True(t, ok)
	assert.Equal(t, uint64(2<<32|123), value)
}

func TestEthtoolCounterFallsBackToLowWord(t *testing.T) {
	stats := map[string]uint64{"counter": 456}

	value, ok := ethtoolCounter(stats, "counter", "counter_h")
	require.True(t, ok)
	assert.Equal(t, uint64(456), value)
}
