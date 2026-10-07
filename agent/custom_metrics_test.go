//go:build testing

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createCustomMetricsAgent returns an agent whose config file, located through
// BESZEL_AGENT_CONFIG, is the given content with {dir} replaced by the
// returned directory for metric files.
func createCustomMetricsAgent(t *testing.T, config string) (*Agent, string) {
	t.Helper()
	metricsDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "agent.yml")
	config = strings.ReplaceAll(config, "{dir}", metricsDir)
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o644))
	t.Setenv("BESZEL_AGENT_CONFIG", configPath)
	t.Setenv("CONFIG", "")
	return createTestAgent(t), metricsDir
}

func writeMetrics(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.prom"), []byte(content), 0o644))
}

func TestCustomMetricsUnconfigured(t *testing.T) {
	t.Setenv("BESZEL_AGENT_CONFIG", "")
	t.Setenv("CONFIG", "")
	agent := createTestAgent(t)

	data := agent.gatherStats(common.DataRequestOptions{CacheTimeMs: defaultDataCacheTimeMs})
	assert.Nil(t, data.Stats.CustomMetrics)
	assert.Nil(t, data.Info.CustomMetricsMeta)

	encoded, err := cbor.Marshal(data)
	require.NoError(t, err)
	var raw map[any]any
	require.NoError(t, cbor.Unmarshal(encoded, &raw))
	assert.NotContains(t, raw[uint64(0)], uint64(48), "Stats key 48 absent")
	assert.NotContains(t, raw[uint64(1)], uint64(30), "Info key 30 absent")
}

func TestCustomMetricsConfigured(t *testing.T) {
	agent, dir := createCustomMetricsAgent(t, `metrics:
  sources:
    - path: {dir}
      display_names:
        pi_power_board_watts: Board power
`)
	writeMetrics(t, dir, "# UNIT pi_power_board_watts watts\npi_power_board_watts 1.84\n")

	data := agent.gatherStats(common.DataRequestOptions{CacheTimeMs: defaultDataCacheTimeMs})
	assert.Equal(t, map[string]float64{"pi_power_board_watts": 1.84}, data.Stats.CustomMetrics)
	assert.Equal(t, map[string]system.CustomMetricMeta{
		"pi_power_board_watts": {Unit: "watts", DisplayName: "Board power", Chart: "Board power"},
	}, data.Info.CustomMetricsMeta)

	encoded, err := cbor.Marshal(data)
	require.NoError(t, err)
	var decoded system.CombinedData
	require.NoError(t, cbor.Unmarshal(encoded, &decoded))
	assert.Equal(t, data.Stats.CustomMetrics, decoded.Stats.CustomMetrics)
	assert.Equal(t, data.Info.CustomMetricsMeta, decoded.Info.CustomMetricsMeta)
}

// The 1s live view and the 60s stored records keep separate counter state,
// so neither computes its change from the other's previous value.
func TestCustomMetricsCountersPerInterval(t *testing.T) {
	agent, dir := createCustomMetricsAgent(t, "metrics:\n  counters: delta\n  sources:\n    - path: {dir}\n")
	counter := func(value string) {
		writeMetrics(t, dir, "# TYPE jobs_total counter\njobs_total "+value+"\n")
	}
	collect := func(cacheTimeMs uint16) map[string]float64 {
		return agent.getSystemStats(cacheTimeMs).CustomMetrics
	}

	counter("100")
	assert.Nil(t, collect(defaultDataCacheTimeMs), "first observation at 60s")
	assert.Nil(t, collect(1000), "first observation at 1s, not a zero delta from the 60s state")

	counter("150")
	assert.Equal(t, map[string]float64{"jobs_total": 50}, collect(1000))
	counter("175")
	assert.Equal(t, map[string]float64{"jobs_total": 25}, collect(1000))
	assert.Equal(t, map[string]float64{"jobs_total": 75}, collect(defaultDataCacheTimeMs), "from the 60s interval's own previous value")
	assert.Equal(t, "jobs_total", agent.systemInfo.CustomMetricsMeta["jobs_total"].DisplayName)
}

// Without a data directory or CONFIG there is no collector. The agent then
// sends neither field, even if metadata was set before.
func TestCustomMetricsNilCollector(t *testing.T) {
	agent := createTestAgent(t)
	agent.customMetrics = nil
	agent.systemInfo.CustomMetricsMeta = map[string]system.CustomMetricMeta{"stale": {Unit: "watts"}}
	var stats system.Stats
	agent.updateCustomMetrics(60000, &stats)
	assert.Nil(t, stats.CustomMetrics)
	assert.Nil(t, agent.systemInfo.CustomMetricsMeta)
}
